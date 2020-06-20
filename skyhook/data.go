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

	// Reads up to n frames.
	// Must only return io.EOF if no Data is returned.
	Read(n int) (Data, error)

	// Like Read but the returned Data will be returned again on the next Read/Peek.
	Peek(n int) (Data, error)

	// sampling frequency of this buffer (1 means no downsampling).
	// For a slice of length N frames, the length if the data from this DataReader
	// should be (N+Freq-1)/Freq (not N/freq because e.g. N=15 Freq=16 should still
	// produce one partial output).
	Freq() int

	Close()
}

type DataBuffer interface {
	Type() DataType
	Reader() DataReader
	Wait() error
}

type DataWriter interface {
	DataBuffer
	Write(data Data)
	Close()
	Error(err error)
}

type SimpleBuffer struct {
	buf Data
	mu sync.Mutex
	cond *sync.Cond
	err error
	done bool
	freq int
	length int
}

type SimpleReader struct {
	buf *SimpleBuffer
	freq int
	pos int
}

func NewSimpleBuffer(t DataType, freq int) *SimpleBuffer {
	if t == VideoType {
		panic(fmt.Errorf("SimpleBuffer should not be used for video"))
	} else if freq <= 0 {
		panic(fmt.Errorf("freq must be positive"))
	}
	buf := &SimpleBuffer{
		buf: NewData(t),
		freq: freq,
	}
	buf.cond = sync.NewCond(&buf.mu)
	return buf
}

// Optionally set the length of this buffer.
// The buffer will drop Writes or duplicate items on Close to ensure it matches the length.
func (buf *SimpleBuffer) SetLength(length int) {
	buf.length = length
}

func (buf *SimpleBuffer) Type() DataType {
	return buf.buf.Type()
}

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
	} else if pos == buf.buf.Length() && buf.done {
		return nil, io.EOF
	}

	if n <= 0 || pos+n > buf.buf.Length() {
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
	if buf.length > 0 && buf.buf.Length() > buf.length {
		buf.buf = buf.buf.Slice(0, buf.length)
	}
	buf.cond.Broadcast()
	buf.mu.Unlock()
}

func (buf *SimpleBuffer) Close() {
	buf.mu.Lock()
	if buf.length > 0 && buf.buf.Length() < buf.length {
		buf.buf = buf.buf.EnsureLength(buf.length)
	}
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
	return &SimpleReader{
		buf: buf,
		freq: buf.freq,
	}
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

func (rd *SimpleReader) Freq() int {
	return rd.freq
}

func (rd *SimpleReader) Close() {}

func MinFreq(inputs []DataReader) int {
	freq := inputs[0].Freq()
	for _, input := range inputs {
		if input.Freq() < freq {
			freq = input.Freq()
		}
	}
	return freq
}

// Duplicate or remove items to adjust the data's freq to target.
// Length is the length at freq 1.
func AdjustDataFreq(data Data, length int, freq int, target int) Data {
	if freq == target {
		return data
	}
	out := NewData(data.Type())
	olen := (length + target-1) / target
	for i := 0; i < olen; i++ {
		idx := i*target/freq
		out = out.Append(data.Slice(idx, idx+1))
	}
	return out
}

// Read from multiple DataReaders in lockstep.
// Output is limited by the slowest DataReader.
// Also adjusts input data so that the Data provided to callback corresponds to targetFreq.
func ReadMultiple(length int, targetFreq int, inputs []DataReader, callback func(int, []Data) error) error {
	defer func() {
		for _, input := range inputs {
			input.Close()
		}
	}()

	// compute the number of frames to read each iteration
	// this depends on the sampling rate since reads should be aligned with the highest freq
	// note that on the last iteration, we may do a partial read on the highest freq readers
	perIter := FPS
	for _, input := range inputs {
		perIter = (perIter / input.Freq()) * input.Freq()
	}
	if perIter == 0 {
		panic(fmt.Errorf("could not find suitable per-iteration read size"))
	}

	completed := 0
	for completed < length {
		// peek each input to see how much we can read
		available := -1
		for _, rd := range inputs {
			data, err := rd.Peek(perIter/rd.Freq())
			if err != nil {
				return err
			}
			if available == -1 || data.Length()*rd.Freq() < available {
				available = data.Length()*rd.Freq()
			}
		}

		datas := make([]Data, len(inputs))
		for i, rd := range inputs {
			// available is always a multiple of Freq due to peeking above except if
			// this is the last data we will read. if that is the case then we want to
			// collect the last partial output from the reader.
			data, err := rd.Read((available+rd.Freq()-1)/rd.Freq())
			if err != nil {
				return err
			}
			datas[i] = AdjustDataFreq(data, available, rd.Freq(), targetFreq)
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
	freq int

	started bool
	dims [2]int
	cmd *Cmd
	stdin io.WriteCloser
}

func NewVideoBuffer(freq int) *VideoBuffer {
	buf := &VideoBuffer{freq: freq}
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
			"ffmpeg-vbuf", CommandOptions{OnlyDebug: true},
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
	rd := &VideoBufferReader{
		buf: buf,
		freq: buf.freq,
	}
	return rd
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
		freq: 1,
	}
}

// This is primarily used as DataReader.
// But it supports the Buffer() and Wait() too so can be used as DataBuffer.
// This is useful in conjunction with the re-scale/re-sample functions.
type VideoBufferReader struct {
	// either (item, slice) or (buf) must be set
	item Item
	slice Slice
	buf *VideoBuffer
	freq int

	rescale [2]int
	resample int
	length int

	cache []Image
	err error
	rd VideoReader
	eof bool
	pos int
}

func (rd *VideoBufferReader) Type() DataType {
	return VideoType
}

func (rd *VideoBufferReader) Rescale(scale [2]int) {
	rd.rescale = scale
}

func (rd *VideoBufferReader) Resample(freq int) {
	rd.resample = freq
}

func (rd *VideoBufferReader) Freq() int {
	freq := rd.freq
	if rd.resample > 0 {
		freq *= rd.resample
	}
	return freq
}

func (rd *VideoBufferReader) SetLength(length int) {
	rd.length = length
}

func (rd *VideoBufferReader) Reader() DataReader {
	return &VideoBufferReader{
		item: rd.item,
		slice: rd.slice,
		buf: rd.buf,
		rescale: rd.rescale,
		resample: rd.resample,
		length: rd.length,
		freq: rd.freq,
	}
}

func (rd *VideoBufferReader) Wait() error {
	if rd.buf == nil {
		// read from file, don't need to wait
		return nil
	}
	return rd.buf.Wait()
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
		sample := 1

		if rd.rescale[0] != 0 {
			width = rd.rescale[0]
			height = rd.rescale[1]
		}
		if rd.resample != 0 {
			sample = rd.resample
		}

		cmd := Command(
			"ffmpeg-vbufrd", CommandOptions{OnlyDebug: true},
			"ffmpeg",
			"-f", "mp4", "-i", "-",
			"-c:v", "rawvideo", "-pix_fmt", "rgb24", "-f", "rawvideo",
			"-vf", fmt.Sprintf("scale=%dx%d,fps=%d/%d", width, height, FPS, sample),
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
		rd.rd = ReadVideo(rd.item, rd.slice, ReadVideoOptions{
			Scale: rd.rescale,
			Sample: rd.resample,
		})
	}
}

func (rd *VideoBufferReader) get(n int, peek bool) (Data, error) {
	if n > FPS {
		panic(fmt.Errorf("n must be <= %d", FPS))
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

	var ims []Image

	// append cached images from previous peek call(s)
	if rd.cache != nil {
		if n > 0 && len(rd.cache) > n {
			ims = rd.cache[0:n]
			rd.cache = append([]Image{}, rd.cache[n:]...)
		} else {
			ims = rd.cache
			rd.cache = nil
		}
	}

	// read from the ffmpeg output
	// we store EOF in case caller peeks and then later reads
	if !rd.eof {
		for len(ims) == 0 || (n > 0 && len(ims) < n) {
			im, err := rd.rd.Read()
			if err == io.EOF {
				rd.eof = true
				break
			} else if err != nil {
				rd.err = err
				return nil, err
			}
			// if ensure-length is set, drop extra frames
			// we still want to keep reading until EOF though
			if rd.length <= 0 || rd.pos+len(ims) < rd.length {
				ims = append(ims, im)
			}
		}
	}

	// if ensure-length is set and we have reached EOF but don't have enough frames,
	// then add extra frames to compensate (but panic if it looks wrong)
	if rd.length > 0 && rd.eof && (len(ims) == 0 || (n > 0 && len(ims) < n)) && rd.pos+len(ims) < rd.length {
		missing := rd.length - (rd.pos+len(ims))
		if missing > 2 {
			panic(fmt.Errorf("too many missing frames"))
		}
		for len(ims) < n && rd.pos+len(ims) < rd.length {
			ims = append(ims, ims[len(ims)-1])
		}
	}

	if rd.eof && len(ims) == 0 {
		return nil, io.EOF
	}

	// either add to the cache or advance pointer
	if peek {
		// prepend since if rd.cache != nil it means we took some out above
		// (we didn't read anything since len(ims) == n)
		rd.cache = append(append([]Image{}, ims...), rd.cache...)
	} else {
		rd.pos += len(ims)
	}

	return VideoData(ims), nil
}

func (rd *VideoBufferReader) Read(n int) (Data, error) {
	return rd.get(n, false)
}

func (rd *VideoBufferReader) Peek(n int) (Data, error) {
	return rd.get(n, true)
}

func (rd *VideoBufferReader) Close() {
	if rd.rd != nil {
		for !rd.eof && rd.err == nil {
			rd.Read(FPS)
		}
		rd.rd.Close()
	}
	rd.err = fmt.Errorf("closed")
}
