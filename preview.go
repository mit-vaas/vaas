package main

import (
	//"github.com/mitroadmaps/gomapinfer/image"

	"bytes"
	"fmt"
	"io"
	"log"
	"sync"
)

var Colors = [][3]uint8{
	[3]uint8{255, 0, 0},
	[3]uint8{0, 255, 0},
	[3]uint8{0, 0, 255},
	[3]uint8{255, 255, 0},
	[3]uint8{0, 255, 255},
	[3]uint8{255, 0, 255},
	[3]uint8{0, 51, 51},
	[3]uint8{51, 153, 153},
	[3]uint8{102, 0, 51},
	[3]uint8{102, 51, 204},
	[3]uint8{102, 153, 204},
	[3]uint8{102, 255, 204},
	[3]uint8{153, 102, 102},
	[3]uint8{204, 102, 51},
	[3]uint8{204, 255, 102},
	[3]uint8{255, 255, 204},
	[3]uint8{121, 125, 127},
	[3]uint8{69, 179, 157},
	[3]uint8{250, 215, 160},
}

// TODO: we can simplify some things like maybe don't even need to store the images here
type PreviewClip struct {
	Slice ClipSlice
	Type DataType

	mu *sync.Mutex
	cond *sync.Cond
	labelBuf *LabelBuffer
	preview *Image
	images []Image
	videoBytes []byte

	// Error while reading images from video source.
	// Or reading labels from labelFunc.
	err error

	// Are we currently populating images[0] / videoBytes?
	loadingPreview bool
	loadingVideo bool

	// images[0:ready] have been modified to show the labels
	// reading images past ready is not thread-safe (even with the lock)
	ready int
}

func DrawLabels(t DataType, data Data, labelOffset int, images []Image) {
	if t == DetectionType || t == TrackType {
		detections := data.Detections
		for i := range images {
			for _, detection := range detections[labelOffset+i] {
				var color [3]uint8
				if t == DetectionType {
					color = [3]uint8{255, 0, 0}
				} else {
					color = Colors[mod(detection.TrackID, len(Colors))]
				}
				images[i].DrawRectangle(detection.Left, detection.Top, detection.Right, detection.Bottom, 2, color)
			}
		}
	}
}

func CreatePreview(slice ClipSlice, t DataType, labelBuf *LabelBuffer) *PreviewClip {
	pc := &PreviewClip{
		Slice: slice,
		Type: t,
		labelBuf: labelBuf,
	}
	pc.mu = new(sync.Mutex)
	pc.cond = sync.NewCond(pc.mu)
	return pc
}

func (pc *PreviewClip) setErr(err error) {
	pc.mu.Lock()
	pc.err = err
	pc.cond.Broadcast()
	pc.mu.Unlock()
}

func (clipLabel *LabeledClip) LoadPreview() *PreviewClip {
	filledBuffer := NewLabelBuffer(clipLabel.Label.Type)
	filledBuffer.Write(clipLabel.Label)
	return CreatePreview(clipLabel.Slice, clipLabel.Type, filledBuffer)
}

func (pc *PreviewClip) GetPreview() (Image, error) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if pc.preview != nil {
		return *pc.preview, nil
	}

	if !pc.loadingPreview {
		pc.loadingPreview = true
		log.Printf("[preview: %v] creating preview image", pc.Slice)
		go func() {
			setPreview := func(im Image) {
				pc.mu.Lock()
				pc.preview = &im
				pc.loadingPreview = false
				pc.cond.Broadcast()
				pc.mu.Unlock()
			}

			data, err := pc.labelBuf.Peek(1)
			if err != nil {
				pc.setErr(err)
				return
			}

			if pc.Type == VideoType {
				images := data.Images
				setPreview(images[0])
				return
			}

			shortSlice := ClipSlice{
				Clip: pc.Slice.Clip,
				Start: pc.Slice.Start,
				End: pc.Slice.Start+1,
			}
			rd := ReadVideo(shortSlice, shortSlice.Clip.Width, shortSlice.Clip.Height)
			defer rd.Close()
			im, err := rd.Read()
			if err != nil {
				pc.setErr(err)
				return
			}
			DrawLabels(pc.Type, data, 0, []Image{im})
			setPreview(im)
		}()
	}

	for pc.preview == nil && pc.err == nil {
		pc.cond.Wait()
	}
	if pc.err != nil {
		return Image{}, pc.err
	}
	return *pc.preview, nil
}

type PCReader struct {
	pc *PreviewClip
	pos int
}

func (rd *PCReader) Read() (Image, error) {
	if rd.pos >= rd.pc.Slice.Length() {
		return Image{}, io.EOF
	}

	rd.pc.mu.Lock()
	defer rd.pc.mu.Unlock()
	for rd.pc.ready <= rd.pos && rd.pc.err == nil {
		rd.pc.cond.Wait()
	}
	if rd.pc.err != nil {
		return Image{}, rd.pc.err
	}
	im := rd.pc.images[rd.pos]
	rd.pos++
	return im, nil
}

func (rd *PCReader) Close() {}

type PCVideoReader struct {
	pc *PreviewClip
	pos int
}

func (rd *PCVideoReader) Read(p []byte) (int, error) {
	rd.pc.mu.Lock()
	defer rd.pc.mu.Unlock()
	for len(rd.pc.videoBytes) <= rd.pos && rd.pc.loadingVideo && rd.pc.err == nil {
		rd.pc.cond.Wait()
	}
	if rd.pc.err != nil {
		return 0, rd.pc.err
	}
	n := copy(p, rd.pc.videoBytes[rd.pos:])
	rd.pos += n
	var err error
	if !rd.pc.loadingVideo {
		err = io.EOF
	}
	return n, err
}

// Helper background thread to load video frames.
// Called by GetVideo.
func (pc *PreviewClip) loadFrames() {
	if pc.Type == VideoType {
		return
	}

	rd := ReadVideo(pc.Slice, pc.Slice.Clip.Width, pc.Slice.Clip.Height)
	defer rd.Close()
	for i := 0; i < pc.Slice.Length(); i++ {
		if i > 0 && i % 100 == 0 {
			log.Printf("[preview: %v] read %d/%d frames", pc.Slice, i, pc.Slice.Length())
		}
		im, err := rd.Read()
		if err != nil {
			pc.setErr(err)
			return
		}
		pc.mu.Lock()
		pc.images = append(pc.images, im)
		pc.cond.Broadcast()
		pc.mu.Unlock()
	}
	log.Printf("[preview: %v] finished reading frames", pc.Slice)
}

// Helper background thread to read and apply labels.
// Called by GetVideo.
func (pc *PreviewClip) loadLabels() {
	// returns at least one image in pc.images[start:end]
	// but may not wait for all of them to load
	getSomeImages := func(start int, end int) ([]Image, error) {
		pc.mu.Lock()
		defer pc.mu.Unlock()
		for len(pc.images) <= start && pc.err == nil {
			pc.cond.Wait()
		}
		if pc.err != nil {
			return nil, pc.err
		}
		if len(pc.images) >= end {
			return pc.images[start:end], nil
		} else {
			return pc.images[start:], nil
		}
	}

	myReady := 0 // our local copy of ready
	updateReady := func() {
		pc.mu.Lock()
		pc.ready = myReady
		pc.cond.Broadcast()
		pc.mu.Unlock()
	}

	for myReady < pc.Slice.Length() {
		data, err := pc.labelBuf.Read(0)
		n := data.Length()
		if err != nil {
			pc.setErr(err)
			return
		} else if n == 0 {
			panic(fmt.Errorf("CreatePreview: got empty label type=%v slice=%v", pc.Type, pc.Slice))
		}

		if pc.Type == VideoType {
			images := data.Images
			pc.mu.Lock()
			pc.images = append(pc.images, images...)
			myReady += n
			pc.ready = myReady
			pc.cond.Broadcast()
			pc.mu.Unlock()
		} else {
			oldReady := myReady
			for myReady < oldReady+n {
				images, err := getSomeImages(myReady, oldReady+n)
				if err != nil {
					pc.setErr(err)
					return
				}
				DrawLabels(pc.Type, data, myReady-oldReady, images)
				myReady += len(images)
				updateReady()
			}
		}
	}
}

func (pc *PreviewClip) GetVideo() (io.Reader, error) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if pc.videoBytes != nil && !pc.loadingVideo {
		return bytes.NewReader(pc.videoBytes), nil
	} else if pc.loadingVideo {
		return &PCVideoReader{pc, 0}, nil
	}

	log.Printf("[preview: %v] convert %d images to video", pc.Slice, pc.Slice.Length())
	pc.loadingVideo = true

	go pc.loadFrames()
	go pc.loadLabels()

	go func() {
		im, err := pc.GetPreview()
		if err != nil {
			return
		}

		rd, cmd := MakeVideo(&PCReader{pc, 0}, im.Width, im.Height)
		buf := make([]byte, 4096)
		for {
			n, err := rd.Read(buf)
			if err == io.EOF {
				cmd.Wait()
				break
			} else if err != nil {
				log.Printf("[preview: %v] GetVideo: error reading from ffmpeg: %v", pc.Slice, err)
				pc.setErr(err)
				return
			}
			pc.mu.Lock()
			pc.videoBytes = append(pc.videoBytes, buf[0:n]...)
			pc.cond.Broadcast()
			pc.mu.Unlock()
		}

		pc.mu.Lock()
		log.Printf("[preview: %v] GetVideo finished getting the video", pc.Slice)
		pc.loadingVideo = false
		pc.cond.Broadcast()
		pc.mu.Unlock()
	}()

	return &PCVideoReader{pc, 0}, nil
}
