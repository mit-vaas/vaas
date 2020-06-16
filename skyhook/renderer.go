package skyhook

import (
	//"github.com/mitroadmaps/gomapinfer/image"

	"bytes"
	"io"
	"log"
	"sync"
)

type VideoRenderer struct {
	slice Slice
	inputs [][]*BufferReader

	// store the non-video labels for inspection
	labels [][]Data

	mu sync.Mutex
	cond *sync.Cond
	preview *Image
	videoBytes []byte

	err error
	done bool
}

func RenderVideo(slice Slice, inputs [][]*BufferReader) *VideoRenderer {
	r := &VideoRenderer{
		slice: slice,
		inputs: inputs,
	}
	r.cond = sync.NewCond(&r.mu)
	go r.render()
	return r
}

func (r *VideoRenderer) setErr(err error) {
	r.mu.Lock()
	r.err = err
	r.cond.Broadcast()
	r.mu.Unlock()
}

func RenderFrames(canvas Image, datas [][]Data, f func(int)) {
	renderOne := func(im Image, data Data, idx int) {
		if data.Type == DetectionType || data.Type == TrackType {
			for _, detection := range data.Detections[idx] {
				var color [3]uint8
				if data.Type == DetectionType {
					color = [3]uint8{255, 0, 0}
				} else {
					color = Colors[Mod(detection.TrackID, len(Colors))]
				}
				im.DrawRectangle(detection.Left, detection.Top, detection.Right, detection.Bottom, 2, color)
			}
		}
	}

	renderFrame := func(vdata Data, datas []Data, idx int) Image {
		im := vdata.Images[idx]
		for _, data := range datas {
			renderOne(im, data, idx)
		}
		return im
	}

	for i := 0; i < datas[0][0].Length(); i++ {
		offset := 0
		for _, l := range datas {
			vdata := l[0]
			if vdata.Type != VideoType {
				continue
			}
			remaining := l[1:]
			im := renderFrame(vdata, remaining, i)
			canvas.DrawImage(0, offset, im)
			offset += im.Height
		}
		f(i)
	}
}

func (r *VideoRenderer) render() {
	ch := make(chan Image)
	var cmd Cmd
	var stdout io.ReadCloser
	var canvas *Image

	labels := make([][]Data, len(r.inputs))
	var flatInputs []*BufferReader
	for i, input := range r.inputs {
		for _, rd := range input {
			labels[i] = append(labels[i], Data{Type: rd.Type()})
			flatInputs = append(flatInputs, rd)
		}
	}

	// start video bytes writer goroutine after MakeVideo call
	donech := make(chan bool)
	writeFunc := func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			r.mu.Lock()
			r.videoBytes = append(r.videoBytes, buf[0:n]...)
			r.cond.Broadcast()
			r.mu.Unlock()
			if err == io.EOF {
				break
			} else if err != nil {
				r.setErr(err)
				break
			}
		}
		stdout.Close()
		cmd.Wait()
		donech <- true
	}

	f := func(index int, flat []Data) error {
		r.mu.Lock()
		if r.err != nil {
			r.mu.Unlock()
			return r.err
		}
		needPreview := r.preview == nil
		r.mu.Unlock()

		datas := make([][]Data, len(r.inputs))
		idx := 0
		for i, input := range r.inputs {
			datas[i] = make([]Data, len(input))
			for j := range input {
				data := flat[idx]
				idx++
				datas[i][j] = data
				if data.Type != VideoType {
					labels[i][j] = labels[i][j].Append(data)
				}
			}
		}

		if canvas == nil {
			width := 0
			height := 0
			for _, l := range datas {
				data := l[0]
				if data.Type != VideoType {
					continue
				}
				height += data.Images[0].Height
				if data.Images[0].Width > width {
					width = data.Images[0].Width
				}
			}
			im := NewImage(width, height)
			canvas = &im

			stdout, cmd = MakeVideo(&chanReader{ch}, width, height)
			go writeFunc()
		}

		RenderFrames(*canvas, datas, func(i int) {
			ch <- *canvas
			if needPreview {
				r.mu.Lock()
				im := canvas.Copy()
				r.preview = &im
				r.cond.Broadcast()
				r.mu.Unlock()
				needPreview = false
				log.Printf("[renderer] set the preview")
			}
		})

		return nil
	}

	ReadMultiple(r.slice.Length(), flatInputs, f)
	close(ch)
	<- donech

	r.mu.Lock()
	r.labels = labels
	r.done = true
	r.cond.Broadcast()
	r.mu.Unlock()
	log.Printf("[renderer] finished rendering video")
}

func (r *VideoRenderer) GetPreview() (Image, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for r.preview == nil && r.err == nil {
		r.cond.Wait()
	}
	if r.err != nil {
		return Image{}, r.err
	}
	return *r.preview, nil
}

type RenderedVideoReader struct {
	r *VideoRenderer
	pos int
}

func (rd *RenderedVideoReader) Read(p []byte) (int, error) {
	rd.r.mu.Lock()
	defer rd.r.mu.Unlock()
	for len(rd.r.videoBytes) <= rd.pos && !rd.r.done && rd.r.err == nil {
		rd.r.cond.Wait()
	}
	if rd.r.err != nil {
		return 0, rd.r.err
	}
	n := copy(p, rd.r.videoBytes[rd.pos:])
	rd.pos += n
	var err error
	if rd.r.done {
		err = io.EOF
	}
	return n, err
}

func (r *VideoRenderer) GetVideo() (io.Reader, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	} else if r.videoBytes != nil && r.done {
		return bytes.NewReader(r.videoBytes), nil
	} else {
		return &RenderedVideoReader{r, 0}, nil
	}
}

func (r *VideoRenderer) GetLabels() ([][]Data, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for !r.done && r.err == nil {
		r.cond.Wait()
	}
	if r.err != nil {
		return nil, r.err
	}
	return r.labels, nil
}
