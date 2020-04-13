package main

import (
	//"github.com/mitroadmaps/gomapinfer/image"

	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
)

const Debug bool = false

type Image struct {
	Width int
	Height int
	//Image [][][3]uint8
	Bytes []byte
}

func ImageFromBytes(width int, height int, bytes []byte) Image {
	/*pix := image.MakeImage(width, height, [3]uint8{0, 0, 0})
	for i, b := range bytes {
		pos := i/3
		channel := i%3
		pix[pos%width][pos/width][channel] = b
	}*/
	return Image{
		Width: width,
		Height: height,
		//Image: pix,
		Bytes: bytes,
	}
}

func ImageFromFile(fname string) Image {
	/*img := image.ReadImage(fname)
	return Image{
		Width: len(img),
		Height: len(img[0]),
		Image: img,
	}*/
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

func (im Image) AsJPG() []byte {
	/*buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, image.AsImage(im.Image), nil); err != nil {
		panic(err)
	}
	return buf.Bytes()*/

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
	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, img, nil); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func (im Image) ToBytes() []byte {
	return im.Bytes
	/*bytes := make([]byte, im.Width*im.Height*3)
	for i := range im.Image {
		for j := range im.Image[i] {
			for channel := 0; channel < 3; channel++ {
				bytes[(j*im.Width+i)*3+channel] = im.Image[i][j][channel]
			}
		}
	}
	return bytes*/
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

	cmd := exec.Command(
	 	"ffmpeg", "-i", fname,
	 	"-ss", ffmpegTime(start), "-to", ffmpegTime(end),
	 	"-c:v", "rawvideo", "-pix_fmt", "rgb24", "-f", "rawvideo",
	 	"-vf", fmt.Sprintf("scale=%dx%d", width, height),
	 	"-",
 	)
 	stdout, err := cmd.StdoutPipe()
 	if err != nil {
 		panic(err)
 	}
 	stderr, err := cmd.StderrPipe()
 	if err != nil {
 		panic(err)
 	}
 	if err := cmd.Start(); err != nil {
 		panic(err)
 	}

 	go func() {
 		rd := bufio.NewReader(stderr)
 		for {
 			line, err := rd.ReadString('\n')
 			if err == io.EOF {
 				break
 			} else if err != nil {
 				panic(err)
 			}
 			if Debug {
 				log.Printf("[ffmpeg] " + strings.TrimSpace(line))
 			}
 		}
 	}()

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

	cmd := exec.Command(
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
	stdin, err := cmd.StdinPipe()
	if err != nil {
		panic(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		panic(err)
	}
	if err := cmd.Start(); err != nil {
		panic(err)
	}

	go printStderr("ffmpeg", stderr, true)

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
