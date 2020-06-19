package skyhook

import (
	"fmt"
	"io"
	"log"
	"sync"
)

type DataType string
const (
	DetectionType DataType = "detection"
	TrackType = "track"
	ClassType = "class"
	VideoType = "video"
)

type DataImpl struct {
	New func() Data
	Decode func([]byte) Data
}

var dataImpls = make(map[DataType]DataImpl)

type Data interface {
	IsEmpty() bool
	Length() int
	EnsureLength(length int) Data
	Slice(i, j int) Data
	Append(other Data) Data
	Encode() []byte
	Type() DataType
}

func NewData(t DataType) Data {
	return dataImpls[t].New()
}

func DecodeData(t DataType, bytes []byte) Data {
	return dataImpls[t].Decode(bytes)
}

type DataReader interface {
	Type() DataType
	Read(n int) (Data, error)
	Peek(n int) (Data, error)
	Close()
}

type DataBuffer interface {
	Type() DataType
	Write(data Data)
	Close()
	Error(err error)
	Reader() DataReader
	Wait() error
}

type SimpleBuffer struct {
	buf Data
	mu sync.Mutex
	cond *sync.Cond
	err error
	done bool
}

type SimpleReader struct {
	buf *SimpleBuffer
	pos int
}

func NewSimpleBuffer(t DataType) *SimpleBuffer {
	if t == VideoType {
		panic(fmt.Errorf("SimpleBuffer should not be used for video"))
	}
	buf := &SimpleBuffer{buf: NewData(t)}
	buf.cond = sync.NewCond(&buf.mu)
	return buf
}

func (buf *SimpleBuffer) Type() DataType {
	return buf.buf.Type()
}

// Read exactly n frames, or any non-zero amount if n<=0
func (buf *SimpleBuffer) read(pos int, n int) (Data, error) {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	wait := n
	if wait <= 0 {
		wait = 1
	}
	for buf.buf.Length() < pos+wait && buf.err == nil && !buf.done {
		buf.cond.Wait()
	}
	if buf.err != nil {
		return nil, buf.err
	} else if buf.buf.Length() == 0 && buf.done {
		return nil, io.EOF
	} else if buf.buf.Length() < n {
		return nil, io.ErrUnexpectedEOF
	}

	if n <= 0 {
		n = buf.buf.Length()-pos
	}
	data := NewData(buf.Type()).Append(buf.buf.Slice(pos, pos+n))
	return data, nil
}

// Wait for at least n to complete, and read them without removing from the buffer.
func (buf *SimpleBuffer) peek(pos int, n int) (Data, error) {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	if n > 0 {
		for buf.buf.Length() < pos+n && buf.err == nil && !buf.done {
			buf.cond.Wait()
		}
	}
	if buf.err != nil {
		return nil, buf.err
	}
	data := NewData(buf.Type()).Append(buf.buf.Slice(pos, buf.buf.Length()))
	return data, nil
}

func (buf *SimpleBuffer) Write(data Data) {
	buf.mu.Lock()
	buf.buf = buf.buf.Append(data)
	buf.cond.Broadcast()
	buf.mu.Unlock()
}

func (buf *SimpleBuffer) Close() {
	buf.mu.Lock()
	buf.done = true
	buf.cond.Broadcast()
	buf.mu.Unlock()
}

func (buf *SimpleBuffer) Error(err error) {
	buf.mu.Lock()
	buf.err = err
	buf.cond.Broadcast()
	buf.mu.Unlock()
}

func (buf *SimpleBuffer) Wait() error {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	for !buf.done && buf.err == nil {
		buf.cond.Wait()
	}
	if buf.err != nil {
		return buf.err
	}
	return nil
}

func (buf *SimpleBuffer) Reader() DataReader {
	return &SimpleReader{buf, 0}
}

func (rd *SimpleReader) Type() DataType {
	return rd.buf.Type()
}

func (rd *SimpleReader) Read(n int) (Data, error) {
	data, err := rd.buf.read(rd.pos, n)
	if data != nil {
		rd.pos += data.Length()
	}
	return data, err
}

func (rd *SimpleReader) Peek(n int) (Data, error) {
	return rd.buf.peek(rd.pos, n)
}

func (rd *SimpleReader) Close() {}

func ReadMultiple(length int, inputs []DataReader, callback func(int, []Data) error) error {
	defer func() {
		for _, input := range inputs {
			input.Close()
		}
	}()

	completed := 0
	for completed < length {
		// peek each input to see how much we can read
		available := -1
		for _, rd := range inputs {
			data, err := rd.Peek(1)
			if err != nil {
				return err
			}
			if available == -1 || data.Length() < available {
				available = data.Length()
			}
		}

		datas := make([]Data, len(inputs))
		for i, rd := range inputs {
			data, err := rd.Read(available)
			if err != nil {
				return err
			}
			datas[i] = data
		}

		err := callback(completed, datas)
		if err != nil {
			return err
		}

		completed += available
	}

	return nil
}

type VideoBuffer struct {
	videoBytes []byte
	mu sync.Mutex
	cond *sync.Cond
	err error
	done bool

	started bool
	dims [2]int
	cmd Cmd
	stdin io.WriteCloser
}

func NewVideoBuffer() *VideoBuffer {
	buf := &VideoBuffer{}
	buf.cond = sync.NewCond(&buf.mu)
	return buf
}

func (buf *VideoBuffer) Type() DataType {
	return VideoType
}

func (buf *VideoBuffer) Write(data Data) {
	images := data.(VideoData)

	buf.mu.Lock()
	if !buf.started {
		buf.dims = [2]int{images[0].Width, images[0].Height}
		buf.cmd = Command(
			"ffmpeg", CommandOptions{OnlyDebug: true},
			"ffmpeg", "-f", "rawvideo", "-framerate", fmt.Sprintf("%v", FPS),
			"-s", fmt.Sprintf("%dx%d", buf.dims[0], buf.dims[1]),
			"-pix_fmt", "rgb24", "-i", "-",
			"-vcodec", "libx264",
			"-vf", fmt.Sprintf("fps=%v", FPS),
			"-f", "mp4", "-movflags", "faststart+frag_keyframe+empty_moov",
			"-",
		)
		buf.started = true
		buf.stdin = buf.cmd.Stdin()

		go func() {
			stdout := buf.cmd.Stdout()
			b := make([]byte, 4096)
			for {
				n, err := stdout.Read(b)
				buf.mu.Lock()
				if n > 0 {
					buf.videoBytes = append(buf.videoBytes, b[0:n]...)
				}
				buf.cond.Broadcast()
				buf.mu.Unlock()
				if err == io.EOF {
					break
				} else if err != nil {
					log.Printf("[VideoBuffer] warning: error encoding video: %v", err)
					break
				}
			}
			stdout.Close()
			buf.cmd.Wait()

			buf.mu.Lock()
			buf.done = true
			buf.cond.Broadcast()
			buf.mu.Unlock()
		}()
	}
	buf.mu.Unlock()

	for _, im := range images {
		buf.stdin.Write(im.ToBytes())
	}
}

func (buf *VideoBuffer) Close() {
	buf.stdin.Close()
	buf.mu.Lock()
	for buf.started && !buf.done && buf.err == nil {
		buf.cond.Wait()
	}
	buf.mu.Unlock()
}

func (buf *VideoBuffer) Error(err error) {
	buf.stdin.Close()
	buf.mu.Lock()
	buf.err = err
	buf.cond.Broadcast()
	buf.mu.Unlock()
}

func (buf *VideoBuffer) Wait() error {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	for !buf.done && buf.err == nil {
		buf.cond.Wait()
	}
	if buf.err != nil {
		return buf.err
	}
	return nil
}

func (buf *VideoBuffer) read(pos int, b []byte) (int, error) {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	for len(buf.videoBytes) <= pos && buf.err == nil && !buf.done {
		buf.cond.Wait()
	}
	if buf.err != nil {
		return 0, buf.err
	} else if len(buf.videoBytes) <= pos && buf.done {
		return 0, io.EOF
	}
	n := copy(b, buf.videoBytes[pos:])
	return n, nil
}

func (buf *VideoBuffer) waitForStarted() error {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	for buf.err == nil && !buf.started {
		buf.cond.Wait()
	}
	if buf.err != nil {
		return buf.err
	}
	return nil
}

func (buf *VideoBuffer) Reader() DataReader {
	return &VideoBufferReader{buf: buf}
}

type VideoFileBuffer struct {
	item Item
	slice Slice
}
func (buf VideoFileBuffer) Type() DataType {
	return VideoType
}
func (buf VideoFileBuffer) Write(data Data) {
	panic(fmt.Errorf("not supported"))
}
func (buf VideoFileBuffer) Close() {}
func (buf VideoFileBuffer) Error(err error) {
	panic(fmt.Errorf("not supported"))
}
func (buf VideoFileBuffer) Wait() error {
	return nil
}
func (buf VideoFileBuffer) Reader() DataReader {
	return &VideoBufferReader{
		item: buf.item,
		slice: buf.slice,
	}
}

type VideoBufferReader struct {
	// either (item, slice) or (buf) must be set
	item Item
	slice Slice
	buf *VideoBuffer

	cache *Image
	err error
	rd VideoReader
}

func (rd *VideoBufferReader) Type() DataType {
	return VideoType
}

func (rd *VideoBufferReader) start() {
	if rd.buf != nil {
		err := rd.buf.waitForStarted()
		if err != nil {
			rd.err = err
			return
		}
		width := rd.buf.dims[0]
		height := rd.buf.dims[1]

		cmd := Command(
			"ffmpeg", CommandOptions{OnlyDebug: true},
			"ffmpeg",
			"-f", "mp4", "-i", "-",
			"-c:v", "rawvideo", "-pix_fmt", "rgb24", "-f", "rawvideo",
			"-vf", fmt.Sprintf("scale=%dx%d", width, height),
			"-",
		)

		go func() {
			stdin := cmd.Stdin()
			b := make([]byte, 4096)
			pos := 0
			for {
				n, err := rd.buf.read(pos, b)
				if err != nil {
					break
				}
				if _, err := stdin.Write(b[0:n]); err != nil {
					break
				}
				pos += n
			}
			stdin.Close()
		}()

		rd.rd = ffmpegReader{
			cmd: cmd,
			stdout: cmd.Stdout(),
			width: width,
			height: height,
			buf: make([]byte, width*height*3),
		}
	} else {
		rd.rd = ReadVideo(rd.item, rd.slice)
	}
}

func (rd *VideoBufferReader) get(n int, peek bool) (Data, error) {
	if n > 1 {
		panic(fmt.Errorf("n must be <=1"))
	}
	if rd.err != nil {
		return nil, rd.err
	}
	if rd.rd == nil {
		rd.start()
		if rd.err != nil {
			return nil, rd.err
		}
	}
	if rd.cache != nil {
		im := *rd.cache
		if !peek {
			rd.cache = nil
		}
		return VideoData{im}, nil
	}
	im, err := rd.rd.Read()
	if err != nil {
		rd.err = err
		return nil, err
	}
	if peek {
		rd.cache = &im
	}
	return VideoData{im}, nil
}

func (rd *VideoBufferReader) Read(n int) (Data, error) {
	return rd.get(n, false)
}

func (rd *VideoBufferReader) Peek(n int) (Data, error) {
	return rd.get(n, true)
}

func (rd *VideoBufferReader) Close() {
	rd.err = fmt.Errorf("closed")
	if rd.rd != nil {
		rd.rd.Close()
	}
}
