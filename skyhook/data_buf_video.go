package skyhook

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"sync"
)

type VideoWriter struct {
	freq int
	cmd *Cmd
	stdin io.WriteCloser
	buf *VideoBuffer
}

func NewVideoWriter() *VideoWriter {
	buf := NewVideoBuffer()
	return &VideoWriter{
		buf: buf,
	}
}

// must be called before any calls to Write
func (w *VideoWriter) SetMeta(freq int) {
	w.freq = freq
}

func (w *VideoWriter) Write(data Data) {
	if w.freq == 0 {
		panic(fmt.Errorf("must SetMeta before Write"))
	}

	images := data.(VideoData)

	if w.stdin == nil {
		width := images[0].Width
		height := images[0].Height
		w.buf.SetMeta(w.freq, width, height)
		w.cmd = Command(
			"ffmpeg-vbuf", CommandOptions{OnlyDebug: true},
			"ffmpeg", "-f", "rawvideo", "-framerate", fmt.Sprintf("%v", FPS),
			"-s", fmt.Sprintf("%dx%d", width, height),
			"-pix_fmt", "rgb24", "-i", "-",
			"-vcodec", "libx264",
			"-vf", fmt.Sprintf("fps=%v", FPS),
			"-f", "mp4", "-movflags", "faststart+frag_keyframe+empty_moov",
			"-",
		)
		w.stdin = w.cmd.Stdin()

		go func() {
			stdout := w.cmd.Stdout()
			_, err := io.Copy(w.buf, stdout)
			if err != nil {
				log.Printf("[VideoWriter] warning: error encoding video: %v", err)
			}
			stdout.Close()
			w.cmd.Wait()
			w.buf.Close()
		}()
	}

	for _, im := range images {
		w.stdin.Write(im.ToBytes())
	}
}

func (w *VideoWriter) Close() {
	w.stdin.Close()
	w.buf.Wait()
}

func (w *VideoWriter) Error(err error) {
	if w.stdin != nil {
		w.stdin.Close()
	}
	w.buf.Error(err)
}

func (w *VideoWriter) Buffer() DataBuffer {
	return w.buf
}

type VideoBuffer struct {
	videoBytes []byte
	mu sync.Mutex
	cond *sync.Cond
	err error
	done bool
	freq int
	dims [2]int

	// whether dims is set yet
	started bool
}

func NewVideoBuffer() *VideoBuffer {
	buf := &VideoBuffer{}
	buf.cond = sync.NewCond(&buf.mu)
	return buf
}

func (buf *VideoBuffer) Type() DataType {
	return VideoType
}

func (buf *VideoBuffer) SetMeta(freq int, width int, height int) {
	buf.mu.Lock()
	buf.freq = freq
	buf.dims = [2]int{width, height}
	buf.started = true
	buf.cond.Broadcast()
	buf.mu.Unlock()
}

func (buf *VideoBuffer) waitForMeta() error {
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

func (buf *VideoBuffer) Write(p []byte) (int, error) {
	buf.mu.Lock()
	buf.videoBytes = append(buf.videoBytes, p...)
	buf.cond.Broadcast()
	buf.mu.Unlock()
	return len(p), nil
}

// this implements io.Closer, not our ones that don't return error
func (buf *VideoBuffer) Close() error {
	buf.mu.Lock()
	buf.done = true
	buf.cond.Broadcast()
	buf.mu.Unlock()
	return nil
}

func (buf *VideoBuffer) Error(err error) {
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

func (buf *VideoBuffer) Reader() DataReader {
	rd := &VideoBufferReader{
		buf: buf,
		freq: buf.freq,
	}
	return rd
}

func (buf *VideoBuffer) Read(pos int, b []byte) (int, error) {
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

func (buf *VideoBuffer) FromReader(r io.Reader) {
	header := make([]byte, 12)
	_, err := io.ReadFull(r, header)
	if err != nil {
		buf.Error(fmt.Errorf("error reading binary: %v", err))
		return
	}
	freq := int(binary.BigEndian.Uint32(header[0:4]))
	width := int(binary.BigEndian.Uint32(header[4:8]))
	height := int(binary.BigEndian.Uint32(header[8:12]))
	buf.SetMeta(freq, width, height)
	_, err = io.Copy(buf, r)
	if err != nil {
		buf.Error(fmt.Errorf("error reading binary: %v", err))
		return
	}
	buf.Close()
}

func (buf *VideoBuffer) ToWriter(w io.Writer) error {
	buf.waitForMeta()
	header := make([]byte, 12)
	binary.BigEndian.PutUint32(header[0:4], uint32(buf.freq))
	binary.BigEndian.PutUint32(header[4:8], uint32(buf.dims[0]))
	binary.BigEndian.PutUint32(header[8:12], uint32(buf.dims[1]))
	w.Write(header)

	pos := 0
	p := make([]byte, 4096)
	for {
		n, err := buf.Read(pos, p)
		if err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
		w.Write(p[0:n])
		pos += n
	}
}

type VideoBinaryReader struct {
	buf *VideoBuffer
	pos int
}

func (r *VideoBinaryReader) Read(p []byte) (int, error) {
	n, err := r.buf.Read(r.pos, p)
	r.pos += n
	return n, err
}

type VideoFileBuffer struct {
	Item Item
	Slice Slice
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
		item: buf.Item,
		slice: buf.Slice,
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
	if freq == 0 {
		rd.buf.waitForMeta()
		rd.freq = rd.buf.freq
		freq = rd.freq
	}
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
		err := rd.buf.waitForMeta()
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
				n, err := rd.buf.Read(pos, b)
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
