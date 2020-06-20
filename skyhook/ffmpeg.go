package skyhook

import (
	//"github.com/mitroadmaps/gomapinfer/image"

	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
)

type Image struct {
	Width int
	Height int
	Bytes []byte
}

func NewImage(width int, height int) Image {
	return Image{
		Width: width,
		Height: height,
		Bytes: make([]byte, 3*width*height),
	}
}

func ImageFromBytes(width int, height int, bytes []byte) Image {
	return Image{
		Width: width,
		Height: height,
		Bytes: bytes,
	}
}

func ImageFromFile(fname string) Image {
	file, err := os.Open(fname)
	if err != nil {
		panic(err)
	}
	im, err := jpeg.Decode(file)
	if err != nil {
		panic(err)
	}
	file.Close()
	rect := im.Bounds()
	width := rect.Dx()
	height := rect.Dy()
	bytes := make([]byte, width*height*3)
	for i := 0; i < width; i++ {
		for j := 0; j < height; j++ {
			r, g, b, _ := im.At(i + rect.Min.X, j + rect.Min.Y).RGBA()
			bytes[(j*width+i)*3+0] = uint8(r >> 8)
			bytes[(j*width+i)*3+1] = uint8(g >> 8)
			bytes[(j*width+i)*3+2] = uint8(b >> 8)
		}
	}
	return Image{
		Width: width,
		Height: height,
		Bytes: bytes,
	}
}

func (im Image) AsImage() image.Image {
	pixbuf := make([]byte, im.Width*im.Height*4)
	j := 0
	channels := 0
	for i := range im.Bytes {
		pixbuf[j] = im.Bytes[i]
		j++
		channels++
		if channels == 3 {
			pixbuf[j] = 255
			j++
			channels = 0
		}
	}
	img := &image.RGBA{
		Pix: pixbuf,
		Stride: im.Width*4,
		Rect: image.Rect(0, 0, im.Width, im.Height),
	}
	return img
}

func (im Image) AsJPG() []byte {
	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, im.AsImage(), nil); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func (im Image) AsPNG() []byte {
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, im.AsImage()); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func (im Image) ToBytes() []byte {
	return im.Bytes
}

func (im Image) Set(i int, j int, color [3]uint8) {
	if i < 0 || i >= im.Width || j < 0 || j >= im.Height {
		return
	}
	for channel := 0; channel < 3; channel++ {
		im.Bytes[(j*im.Width+i)*3+channel] = color[channel]
	}
}

func (im Image) Get(i int, j int) [3]uint8 {
	var color [3]uint8
	for channel := 0; channel < 3; channel++ {
		color[channel] = im.Bytes[(j*im.Width+i)*3+channel]
	}
	return color
}

func (im Image) FillRectangle(left, top, right, bottom int, color [3]uint8) {
	for i := left; i < right; i++ {
		for j := top; j < bottom; j++ {
			im.Set(i, j, color)
		}
	}
}

func (im Image) Copy() Image {
	bytes := make([]byte, len(im.Bytes))
	copy(bytes, im.Bytes)
	return Image{
		Width: im.Width,
		Height: im.Height,
		Bytes: bytes,
	}
}

func (im Image) DrawRectangle(left, top, right, bottom int, width int, color [3]uint8) {
	im.FillRectangle(left-width, top, left+width, bottom, color)
	im.FillRectangle(right-width, top, right+width, bottom, color)
	im.FillRectangle(left, top-width, right, top+width, color)
	im.FillRectangle(left, bottom-width, right, bottom+width, color)
}

func (im Image) DrawImage(left int, top int, other Image) {
	for i := 0; i < other.Width; i++ {
		for j := 0; j < other.Height; j++ {
			im.Set(left+i, top+j, other.Get(i, j))
		}
	}
}

type VideoReader interface {
	// Error should be io.EOF if there are no more images.
	// If an image is returned, error must NOT be io.EOF.
	// (So no error should be returned on the last image, only after the last image.)
	Read() (Image, error)

	Close()
}

type ffmpegReader struct {
	cmd *Cmd
	stdout io.ReadCloser
	width int
	height int
	buf []byte
}

func ffmpegTime(index int) string {
	ts := (index*100)/FPS
	return fmt.Sprintf("%d.%02d", ts/100, ts%100)
}

// Returns time in milliseconds e.g. "01:32:42.1" -> (1*3600+32*60+42.1)*1000.
func parseFfmpegTime(str string) int {
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

func ReadFfmpeg(fname string, start int, end int, opts ReadVideoOptions) ffmpegReader {
	log.Printf("[ffmpeg] from %s extract frames [%d:%d) %dx%d", fname, start, end, opts.Scale[0], opts.Scale[1])

	cmd := Command(
		"ffmpeg-read", CommandOptions{NoStdin: true, OnlyDebug: true},
		"ffmpeg",
		"-ss", ffmpegTime(start),
		"-i", fname,
		"-to", ffmpegTime(end-start),
		"-c:v", "rawvideo", "-pix_fmt", "rgb24", "-f", "rawvideo",
		"-vf", fmt.Sprintf("scale=%dx%d,fps=%d/%d", opts.Scale[0], opts.Scale[1], FPS, opts.Sample),
		"-",
	)

	return ffmpegReader{
		cmd: cmd,
		stdout: cmd.Stdout(),
		width: opts.Scale[0],
		height: opts.Scale[1],
		buf: make([]byte, opts.Scale[0]*opts.Scale[1]*3),
	}
}

func (rd ffmpegReader) Read() (Image, error) {
	_, err := io.ReadFull(rd.stdout, rd.buf)
	if err != nil {
		return Image{}, err
	}
	buf := make([]byte, len(rd.buf))
	copy(buf, rd.buf)
	return ImageFromBytes(rd.width, rd.height, buf), nil
}

func (rd ffmpegReader) Close() {
	rd.stdout.Close()
	rd.cmd.Wait()
}

type sliceReader struct {
	images []Image
	pos int
}
func (rd *sliceReader) Read() (Image, error) {
	if rd.pos >= len(rd.images) {
		return Image{}, io.EOF
	}
	im := rd.images[rd.pos]
	rd.pos++
	return im, nil
}
func (rd *sliceReader) Close() {}

type chanReader struct {
	ch chan Image
}
func (rd *chanReader) Read() (Image, error) {
	im, ok := <- rd.ch
	if !ok {
		return Image{}, io.EOF
	}
	return im, nil
}
func (rd *chanReader) Close() {
	go func() {
		for _ = range rd.ch {}
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
		"ffmpeg", "-f", "rawvideo",
		"-s", fmt.Sprintf("%dx%d", width, height),
		//"-r", fmt.Sprintf("%v", FPS),
		"-pix_fmt", "rgb24", "-i", "-",
		"-vcodec", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-g", fmt.Sprintf("%v", FPS),
		//"-vcodec", "mpeg4",
		"-vf", fmt.Sprintf("fps=%v", FPS),
		//"-f", "flv",
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
