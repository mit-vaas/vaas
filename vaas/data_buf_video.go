package vaas

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

/*

VideoBuffer stores encoded video bytes.
More bytes can be added in a streaming way.

VideoBuffer is used in two ways:

First, VideoWriter writes an image sequence to a VideoBuffer by encoding the
image sequence as video through ffmpeg. This is used e.g. to store video
outputs of executors.

Second, VideoBufferFromReader takes a binary IO stream containing video bytes
and writes it to a VideoBuffer.

VideoFileBuffer is an alternative DataBuffer for video data when the video is
already stored on disk. It just stores the file metadata.

VideoBufferReader is a DataReader for video data that reads from either a
VideoBuffer or a VideoFileBuffer. It decodes the video and returns it as an
image sequence. It supports push-down of rescale and re-sample operations.

VideoBufferReader also supports a ReadMP4 to read the VideoBuffer or
VideoFileBuffer directly as an MP4 file. Ordinarily reading the bytes directly
from the file or the VideoBuffer would be sufficient. However, if rescale or
re-sample was pushed down, then we need to transcode that video. So that is why
ReadMP4 exists.

*/

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

func (buf *VideoBuffer) ToWriter(w io.Writer) error {
	return buf.Reader().(*VideoBufferReader).ToWriter(w)
}

// unlike ToWriter, this writes the video data directly without metadata
func (buf *VideoBuffer) writeBuffer(w io.Writer) error {
	p := make([]byte, 4096)
	pos := 0
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

type MP4Reader interface {
	ReadMP4(io.Writer) error
}

func (buf *VideoBuffer) ReadMP4(w io.Writer) error {
	return buf.Reader().(*VideoBufferReader).ReadMP4(w)
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

func (rd *VideoBufferReader) getDims() [2]int {
	if rd.rescale[0] != 0 {
		return rd.rescale
	} else if rd.buf != nil {
		return rd.buf.dims
	} else {
		return [2]int{rd.item.Width, rd.item.Height}
	}
}

func (rd *VideoBufferReader) getSample() int {
	if rd.resample == 0 {
		return 1
	}
	return rd.resample
}

func (rd *VideoBufferReader) start() {
	if rd.buf != nil {
		err := rd.buf.waitForMeta()
		if err != nil {
			rd.err = err
			return
		}
		dims := rd.getDims()
		sample := rd.getSample()

		cmd := Command(
			"ffmpeg-vbufrd", CommandOptions{OnlyDebug: true},
			"ffmpeg",
			"-f", "mp4", "-i", "-",
			"-c:v", "rawvideo", "-pix_fmt", "rgb24", "-f", "rawvideo",
			"-vf", fmt.Sprintf("scale=%dx%d,fps=%d/%d", dims[0], dims[1], FPS, sample),
			"-",
		)

		go func() {
			stdin := cmd.Stdin()
			rd.buf.writeBuffer(stdin)
			stdin.Close()
		}()

		rd.rd = FfmpegReader{
			Cmd: cmd,
			Stdout: cmd.Stdout(),
			Width: dims[0],
			Height: dims[1],
			Buf: make([]byte, dims[0]*dims[1]*3),
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
		rd.rd.Close()
	}
	rd.err = fmt.Errorf("closed")
}

type VideoIOMeta struct {
	Resample int
	Rescale [2]int

	BufFreq int
	BufWidth int
	BufHeight int

	Item *Item
	Slice Slice
}

func VideoBufferFromReader(r io.ReadCloser) DataBuffer {
	hlen := make([]byte, 4)
	_, err := io.ReadFull(r, hlen)
	if err != nil {
		return GetErrorBuffer(VideoType, fmt.Errorf("error reading video from io.Reader: %v", err))
	}
	l := int(binary.BigEndian.Uint32(hlen))
	metaBytes := make([]byte, l)
	_, err = io.ReadFull(r, metaBytes)
	if err != nil {
		return GetErrorBuffer(VideoType, fmt.Errorf("error reading video from io.Reader: %v", err))
	}
	var meta VideoIOMeta
	JsonUnmarshal(metaBytes, &meta)

	if meta.Item == nil {
		// reader sends the video bytes
		buf := NewVideoBuffer()
		buf.SetMeta(meta.BufFreq, meta.BufWidth, meta.BufHeight)
		rd := buf.Reader().(*VideoBufferReader)
		rd.resample = meta.Resample
		rd.rescale = meta.Rescale
		go func() {
			_, err = io.Copy(buf, r)
			if err != nil {
				buf.Error(fmt.Errorf("error reading video (VideoBuffer) from io.Reader: %v", err))
				return
			}
			r.Close()
			buf.Close()
		}()
		return rd
	} else {
		r.Close()
		return &VideoBufferReader{
			item: *meta.Item,
			slice: meta.Slice,
			freq: meta.Item.Freq,
			resample: meta.Resample,
			rescale: meta.Rescale,
		}
	}
}

func (rd *VideoBufferReader) ToWriter(w io.Writer) error {
	meta := VideoIOMeta{
		Resample: rd.resample,
		Rescale: rd.rescale,
	}

	writeMeta := func() {
		metaBytes := JsonMarshal(meta)
		hlen := make([]byte, 4)
		binary.BigEndian.PutUint32(hlen, uint32(len(metaBytes)))
		w.Write(hlen)
		w.Write(metaBytes)
	}

	if rd.buf != nil {
		rd.buf.waitForMeta()
		meta.BufFreq = rd.buf.freq
		meta.BufWidth = rd.buf.dims[0]
		meta.BufHeight = rd.buf.dims[1]
		writeMeta()
		return rd.buf.writeBuffer(w)
	} else {
		item := rd.item
		meta.Item = &item
		meta.Slice = rd.slice
		writeMeta()
	}

	return nil
}

// Read rd.buf or rd.item as an MP4 instead of an image sequence.
// If rescale/resample are not set or match the input, then we can just return
// the stored bytes directly. Otherwise, we need to transcade the video.
func (rd *VideoBufferReader) ReadMP4(w io.Writer) error {
	if rd.buf != nil {
		rd.buf.waitForMeta()
		needTranscode := false
		if rd.rescale[0] != 0 && rd.rescale != rd.buf.dims {
			needTranscode = true
		}
		if rd.resample != 0 && rd.resample > 1 {
			needTranscode = true
		}
		if !needTranscode {
			return rd.buf.writeBuffer(w)
		}

		dims := rd.getDims()
		sample := rd.getSample()

		cmd := Command(
			"ffmpeg-vbufrd", CommandOptions{OnlyDebug: true},
			"ffmpeg",
			"-f", "mp4", "-i", "-",
			"-vcodec", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-g", fmt.Sprintf("%v", FPS),
			"-vf", fmt.Sprintf("scale=%dx%d,fps=%d/%d", dims[0], dims[1], FPS, sample),
			"-f", "mp4", "-pix_fmt", "yuv420p", "-movflags", "faststart+frag_keyframe+empty_moov",
			"-",
		)

		go func() {
			stdin := cmd.Stdin()
			rd.buf.writeBuffer(stdin)
			stdin.Close()
		}()
		io.Copy(w, cmd.Stdout())
		return cmd.Wait()
	} else {
		needTranscode := false
		if rd.rescale[0] != 0 && rd.rescale != [2]int{rd.item.Width, rd.item.Height} {
			needTranscode = true
		}
		if rd.resample != 0 && rd.resample > 1 {
			needTranscode = true
		}
		if !needTranscode {
			file, err := os.Open(rd.item.Fname(0))
			if err != nil {
				return err
			}
			_, err = io.Copy(w, file)
			file.Close()
			return err
		}

		dims := rd.getDims()
		sample := rd.getSample()

		cmd := Command(
			"ffmpeg-vbufrd", CommandOptions{NoStdin: true, OnlyDebug: true},
			"ffmpeg",
			"-ss", ffmpegTime(rd.slice.Start - rd.item.Slice.Start),
			"-i", rd.item.Fname(0),
			"-to", ffmpegTime(rd.slice.Length()),
			"-vf", fmt.Sprintf("scale=%dx%d,fps=%d/%d", dims[0], dims[1], FPS, sample),
			"-f", "mp4", "-pix_fmt", "yuv420p", "-movflags", "faststart+frag_keyframe+empty_moov",
			"-",
		)

		go func() {
			stdin := cmd.Stdin()
			rd.buf.writeBuffer(stdin)
			stdin.Close()
		}()
		io.Copy(w, cmd.Stdout())
		return cmd.Wait()
	}
}
