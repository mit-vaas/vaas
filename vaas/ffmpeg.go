package vaas

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
)

type VideoReader interface {
	// Error should be io.EOF if there are no more images.
	// If an image is returned, error must NOT be io.EOF.
	// (So no error should be returned on the last image, only after the last image.)
	Read() (Image, error)

	Close()
}

type FfmpegReader struct {
	Cmd *Cmd
	Stdout io.ReadCloser
	Width int
	Height int
	Buf []byte
	ExpectedFrames int
	count int
	last Image
}

func ffmpegTime(index int) string {
	ts := (index*100)/FPS
	return fmt.Sprintf("%d.%02d", ts/100, ts%100)
}

// Returns time in milliseconds e.g. "01:32:42.1" -> (1*3600+32*60+42.1)*1000.
func ParseFfmpegTime(str string) int {
	parts := strings.Split(str, ":")
	if len(parts) == 3 {
		h := ParseFloat(parts[0])
		m := ParseFloat(parts[1])
		s := ParseFloat(parts[2])
		return int(h*3600*1000 + m*60*1000 + s*1000)
	} else {
		s := ParseFloat(str)
		return int(s*1000)
	}
}

func ReadFfmpeg(fname string, start int, end int, opts ReadVideoOptions) *FfmpegReader {
	log.Printf("[ffmpeg] from %s extract frames [%d:%d) %dx%d", fname, start, end, opts.Scale[0], opts.Scale[1])

	nframes := (end-start+opts.Sample-1)/opts.Sample
	cmd := Command(
		"ffmpeg-read", CommandOptions{NoStdin: true, OnlyDebug: true},
		"ffmpeg",
		"-threads", "2",
		"-ss", ffmpegTime(start),
		"-i", fname,
		//"-to", ffmpegTime(end-start),
		"-vframes", fmt.Sprintf("%d", nframes),
		"-c:v", "rawvideo", "-pix_fmt", "rgb24", "-f", "rawvideo",
		"-vf", fmt.Sprintf("scale=%dx%d,fps=fps=%d/%d:round=up", opts.Scale[0], opts.Scale[1], FPS, opts.Sample),
		"-",
	)

	return &FfmpegReader{
		Cmd: cmd,
		Stdout: cmd.Stdout(),
		Width: opts.Scale[0],
		Height: opts.Scale[1],
		Buf: make([]byte, opts.Scale[0]*opts.Scale[1]*3),
		ExpectedFrames: nframes,
	}
}

func (rd *FfmpegReader) Read() (Image, error) {
	_, err := io.ReadFull(rd.Stdout, rd.Buf)
	if err == io.EOF {
		if rd.count == 0 || rd.count >= rd.ExpectedFrames {
			return Image{}, err
		} else {
			rd.count++
			return rd.last, nil
		}
	} else if err != nil {
		return Image{}, err
	}
	buf := make([]byte, len(rd.Buf))
	copy(buf, rd.Buf)
	im := ImageFromBytes(rd.Width, rd.Height, buf)
	rd.last = im
	rd.count++
	return im, nil
}

func (rd *FfmpegReader) Close() {
	rd.Stdout.Close()
	rd.Cmd.Wait()
}

type SliceReader struct {
	Images []Image
	Pos int
}
func (rd *SliceReader) Read() (Image, error) {
	if rd.Pos >= len(rd.Images) {
		return Image{}, io.EOF
	}
	im := rd.Images[rd.Pos]
	rd.Pos++
	return im, nil
}
func (rd *SliceReader) Close() {}

type ChanReader struct {
	Ch chan Image
}
func (rd *ChanReader) Read() (Image, error) {
	im, ok := <- rd.Ch
	if !ok {
		return Image{}, io.EOF
	}
	return im, nil
}
func (rd *ChanReader) Close() {
	go func() {
		for _ = range rd.Ch {}
	}()
}

type ReadVideoOptions struct {
	Scale [2]int
	Sample int
}

func ReadVideo(item Item, slice Slice, opts ReadVideoOptions) VideoReader {
	if opts.Scale[0] == 0 {
		opts.Scale = [2]int{item.Width, item.Height}
	}
	if opts.Sample == 0 {
		opts.Sample = 1
	}
	if item.Format == "jpeg" {
		return ReadJpegParallel(item, slice.Start - item.Slice.Start, slice.End - item.Slice.Start, 4, opts)
	} else {
		return ReadFfmpeg(item.Fname(0), slice.Start - item.Slice.Start, slice.End - item.Slice.Start, opts)
	}
}

func MakeVideo(rd VideoReader, width int, height int) (io.ReadCloser, *Cmd) {
	log.Printf("[ffmpeg] make video (%dx%d)", width, height)

	cmd := Command(
		"ffmpeg-mkvid", CommandOptions{OnlyDebug: true},
		"ffmpeg",
		"-threads", "2",
		"-f", "rawvideo",
		"-s", fmt.Sprintf("%dx%d", width, height),
		//"-r", fmt.Sprintf("%v", FPS),
		"-pix_fmt", "rgb24", "-i", "-",
		"-vcodec", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-g", fmt.Sprintf("%v", FPS),
		"-vf", fmt.Sprintf("fps=fps=%v", FPS),
		"-f", "mp4", "-pix_fmt", "yuv420p", "-movflags", "faststart+frag_keyframe+empty_moov",
		"-",
	)

	go func() {
		stdin := cmd.Stdin()
		for {
			im, err := rd.Read()
			if err == io.EOF {
				break
			} else if err != nil {
				log.Printf("[ffmpeg] error making video: %v", err)
				break
			}
			_, err = stdin.Write(im.ToBytes())
			if err != nil {
				log.Printf("[ffmpeg] error making video: %v", err)
				break
			}
		}
		stdin.Close()
	}()

	return cmd.Stdout(), cmd
}

func Ffprobe(fname string) (width int, height int, duration float64, err error) {
	cmd := Command(
		"ffprobe", CommandOptions{NoStdin: true},
		"ffprobe",
		"-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height,duration",
		"-of", "csv=s=,:p=0",
		fname,
	)
	rd := bufio.NewReader(cmd.Stdout())
	var line string
	line, err = rd.ReadString('\n')
	if err != nil {
		return
	}
	parts := strings.Split(strings.TrimSpace(line), ",")
	width, _ = strconv.Atoi(parts[0])
	height, _ = strconv.Atoi(parts[1])
	duration, _ = strconv.ParseFloat(parts[2], 64)
	cmd.Wait()
	return
}
