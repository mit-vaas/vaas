package skyhook

import (
	//"github.com/mitroadmaps/gomapinfer/image"

	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"os"
	"os/exec"
)

type Image struct {
	Width int
	Height int
	Bytes []byte
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

func (im Image) FillRectangle(left, top, right, bottom int, color [3]uint8) {
	for i := left; i < right; i++ {
		for j := top; j < bottom; j++ {
			if i < 0 || i >= im.Width || j < 0 || j >= im.Height {
				continue
			}
			for channel := 0; channel < 3; channel++ {
				im.Bytes[(j*im.Width+i)*3+channel] = color[channel]
			}
		}
	}
}

func (im Image) DrawRectangle(left, top, right, bottom int, width int, color [3]uint8) {
	im.FillRectangle(left-width, top, left+width, bottom, color)
	im.FillRectangle(right-width, top, right+width, bottom, color)
	im.FillRectangle(left, top-width, right, top+width, color)
	im.FillRectangle(left, bottom-width, right, bottom+width, color)
}

type VideoReader interface {
	// Error should be io.EOF if there are no more images.
	// If an image is returned, error must NOT be io.EOF.
	// (So no error should be returned on the last image, only after the last image.)
	Read() (Image, error)

	Close()
}

type ffmpegReader struct {
	cmd *exec.Cmd
	stdout io.ReadCloser
	width int
	height int
	buf []byte
}

func ffmpegTime(index int) string {
	ts := (index*100)/FPS
	return fmt.Sprintf("%d.%d", ts/100, ts%100)
}

func ReadFfmpeg(fname string, start int, end int, width int, height int) ffmpegReader {
	log.Printf("[ffmpeg] from %s extract frames [%d:%d) %dx%d", fname, start, end, width, height)

	cmd, _, stdout := Command(
		"ffmpeg", true,
	 	"ffmpeg", "-i", fname,
	 	"-ss", ffmpegTime(start), "-to", ffmpegTime(end),
	 	"-c:v", "rawvideo", "-pix_fmt", "rgb24", "-f", "rawvideo",
	 	"-vf", fmt.Sprintf("scale=%dx%d", width, height),
	 	"-",
 	)

 	return ffmpegReader{
 		cmd: cmd,
 		stdout: stdout,
 		width: width,
 		height: height,
 		buf: make([]byte, width*height*3),
 	}
}

func (rd ffmpegReader) Read() (Image, error) {
	_, err := io.ReadFull(rd.stdout, rd.buf)
	if err != nil {
		rd.Close()
		return Image{}, err
	}
	return ImageFromBytes(rd.width, rd.height, rd.buf), nil
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

func ReadVideo(slice ClipSlice, width int, height int) VideoReader {
	if slice.Clip.Video.Ext == "jpeg" {
		return ReadJpegParallel(slice.Clip, slice.Start, slice.End, 4)
	} else {
		return ReadFfmpeg(slice.Clip.Fname(slice.Start), slice.Start, slice.End, width, height)
	}
}

func GetFrames(slice ClipSlice, width int, height int) ([]Image, error) {
	rd := ReadVideo(slice, width, height)
	var frames []Image
	for {
		im, err := rd.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		frames = append(frames, im)
	}
	return frames, nil
}

func MakeVideo(rd VideoReader, width int, height int) (io.ReadCloser, *exec.Cmd) {
	log.Printf("[ffmpeg] make video (%dx%d)", width, height)

	cmd, stdin, stdout := Command(
		"ffmpeg", true,
		"ffmpeg", "-f", "rawvideo",
		"-s", fmt.Sprintf("%dx%d", width, height),
		"-pix_fmt", "rgb24", "-i", "-",
		"-vcodec", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-g", fmt.Sprintf("%v", FPS),
		//"-vcodec", "mpeg4",
		"-vf", fmt.Sprintf("fps=%v", FPS),
		//"-f", "flv",
		"-f", "mp4", "-pix_fmt", "yuv420p", "-movflags", "faststart+frag_keyframe+empty_moov",
		"-",
	)

	go func() {
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

	return stdout, cmd
}
