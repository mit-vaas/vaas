package main

import (
	"github.com/mitroadmaps/gomapinfer/image"

	"bufio"
	"bytes"
	"fmt"
	"image/jpeg"
	"io"
	"log"
	"os/exec"
	"strings"
)

const Debug bool = false

type Image struct {
	Width int
	Height int
	Image [][][3]uint8
}

func ImageFromBytes(width int, height int, bytes []byte) Image {
	pix := image.MakeImage(width, height, [3]uint8{0, 0, 0})
	for i, b := range bytes {
		pos := i/3
		channel := i%3
		pix[pos%width][pos/width][channel] = b
	}
	return Image{
		Width: width,
		Height: height,
		Image: pix,
	}
}

func ImageFromFile(fname string) Image {
	img := image.ReadImage(fname)
	return Image{
		Width: len(img),
		Height: len(img[0]),
		Image: img,
	}
}

func (im Image) AsJPG() []byte {
	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, image.AsImage(im.Image), nil); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func (im Image) ToBytes() []byte {
	bytes := make([]byte, im.Width*im.Height*3)
	for i := range im.Image {
		for j := range im.Image[i] {
			for channel := 0; channel < 3; channel++ {
				bytes[(j*im.Width+i)*3+channel] = im.Image[i][j][channel]
			}
		}
	}
	return bytes
}

type videoReader struct {
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

func VideoReader(fname string, start int, end int, width int, height int) videoReader {
	log.Printf("[ffmpeg] from %s extract frames [%d:%d] %dx%d", fname, start, end, width, height)

	cmd := exec.Command(
	 	"ffmpeg", "-i", fname,
	 	"-ss", ffmpegTime(start), "-to", ffmpegTime(end+1),
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

 	return videoReader{
 		cmd: cmd,
 		stdout: stdout,
 		width: width,
 		height: height,
 		buf: make([]byte, width*height*3),
 	}
}

func (rd videoReader) Read() (Image, bool) {
	_, err := io.ReadFull(rd.stdout, rd.buf)
	if err == io.EOF {
		return Image{}, true
	} else if err != nil {
		panic(err)
	}
	return ImageFromBytes(rd.width, rd.height, rd.buf), false
}

func (rd videoReader) Close() {
	rd.stdout.Close()
	rd.cmd.Wait()
}

func GetFrames(clip Clip, start int, end int, width int, height int) []Image {
	if clip.Video.Ext == "jpeg" {
		var frames []Image
		for i := start; i <= end; i++ {
			im := ImageFromFile(clip.Fname(i))
			frames = append(frames, im)
		}
		return frames
	} else {
		rd := VideoReader(clip.Fname(start), start, end, width, height)
		var frames []Image
		for {
			im, done := rd.Read()
			if done {
				break
			}
			frames = append(frames, im)
		}
		return frames
	}
}

func MakeVideo(images []Image) []byte {
	log.Printf("[ffmpeg] make video from %s frames (%dx%d)", len(images), images[0].Width, images[0].Height)

	cmd := exec.Command(
		"ffmpeg", "-f", "rawvideo",
		"-s", fmt.Sprintf("%dx%d", images[0].Width, images[0].Height),
		"-pix_fmt", "rgb24", "-i", "-",
		"-vcodec", "libx264", "-f", "mp4", "-pix_fmt", "yuv420p",
		"-vf", fmt.Sprintf("fps=%v", FPS),
		"-movflags", "frag_keyframe+empty_moov",
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

	go printStderr("ffmpeg", stderr)

	go func() {
		for _, im := range images {
			_, err := stdin.Write(im.ToBytes())
			if err != nil {
				log.Printf("[ffmpeg] error making video: %v", err)
				break
			}
		}
		stdin.Close()
	}()

	data := new(bytes.Buffer)
	buf := make([]byte, 4096)
	for {
		n, err := stdout.Read(buf)
		if err == io.EOF {
			break
		} else if err != nil {
			log.Printf("[ffmpeg] error making video: %v", err)
			break
		}
		data.Write(buf[0:n])
	}
	stdin.Close()
	stdout.Close()

	return data.Bytes()
}
