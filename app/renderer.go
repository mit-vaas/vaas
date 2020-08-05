package app

import (
	"../vaas"

	"bytes"
	"io"
	"log"
	"sync"
)

type VideoRenderer struct {
	slice vaas.Slice
	inputs [][]vaas.DataReader
	opts RenderOpts

	// store the non-video labels for inspection
	labels [][]vaas.Data

	mu sync.Mutex
	cond *sync.Cond
	preview *vaas.Image
	videoBytes []byte

	err error
	done bool
}

type RenderOpts struct {
	ProgressCallback func(progress int)
}

func RenderVideo(slice vaas.Slice, inputs [][]vaas.DataReader, opts RenderOpts) *VideoRenderer {
	r := &VideoRenderer{
		slice: slice,
		inputs: inputs,
		opts: opts,
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

func RenderFrames(canvas vaas.Image, datas [][]vaas.Data, f func(int)) {
	renderOne := func(im vaas.Image, data vaas.Data, idx int) {
		if data.Type() == vaas.DetectionType || data.Type() == vaas.TrackType {
			var detections [][]vaas.Detection
			if data.Type() == vaas.DetectionType {
				detections = [][]vaas.Detection(data.(vaas.DetectionData))
			} else {
				detections = [][]vaas.Detection(data.(vaas.TrackData))
			}
			for _, detection := range detections[idx] {
				var color [3]uint8
				if data.Type() == vaas.DetectionType {
					color = [3]uint8{255, 0, 0}
				} else {
					color = Colors[vaas.Mod(detection.TrackID, len(Colors))]
				}
				im.DrawRectangle(detection.Left, detection.Top, detection.Right, detection.Bottom, 2, color)
			}
		} else if data.Type() == vaas.TextType {
			texts := data.(vaas.TextData)
			im.DrawText(texts[idx])
		} else if data.Type() == vaas.ImListType {
			imlist := data.(vaas.ImListData)[idx]
			// draw the images one by one horizontally over the canvas
			pos := 0
			for _, el := range imlist {
				im.DrawImage(pos, 0, el)
				pos += el.Width
			}
		}
	}

	renderFrame := func(vdata vaas.VideoData, datas []vaas.Data, idx int) vaas.Image {
		im := vdata[idx]
		for _, data := range datas {
			renderOne(im, data, idx)
		}
		return im
	}

	for i := 0; i < datas[0][0].Length(); i++ {
		offset := 0
		for _, l := range datas {
			head := l[0]
			if head.Type() != vaas.VideoType {
				continue
			}
			vdata := head.(vaas.VideoData)
			remaining := l[1:]
			im := renderFrame(vdata, remaining, i)
			canvas.DrawImage(0, offset, im)
			offset += im.Height
		}
		f(i)
	}
}

func (r *VideoRenderer) render() {
	ch := make(chan vaas.Image)
	var cmd *vaas.Cmd
	var stdout io.ReadCloser
	var canvas *vaas.Image

	labels := make([][]vaas.Data, len(r.inputs))
	var flatInputs []vaas.DataReader
	for i, input := range r.inputs {
		for _, rd := range input {
			labels[i] = append(labels[i], vaas.NewData(rd.Type()))
			flatInputs = append(flatInputs, rd)
		}
	}

	// start video bytes writer goroutine after MakeVideo call
	var donech chan bool
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

	prevProgress := 0
	f := func(index int, flat []vaas.Data) error {
		r.mu.Lock()
		if r.err != nil {
			r.mu.Unlock()
			return r.err
		}
		needPreview := r.preview == nil
		r.mu.Unlock()

		if r.opts.ProgressCallback != nil && index - prevProgress >= vaas.FPS {
			prevProgress = index
			r.opts.ProgressCallback(100*index/r.slice.Length())
		}

		datas := make([][]vaas.Data, len(r.inputs))
		idx := 0
		for i, input := range r.inputs {
			datas[i] = make([]vaas.Data, len(input))
			for j := range input {
				data := flat[idx]
				idx++
				datas[i][j] = data
				if data.Type() != vaas.VideoType {
					labels[i][j] = labels[i][j].Append(data)
				}
			}
		}

		if canvas == nil {
			width := 0
			height := 0
			for _, l := range datas {
				data := l[0]
				if data.Type() != vaas.VideoType {
					continue
				}
				vdata := data.(vaas.VideoData)
				height += vdata[0].Height
				if vdata[0].Width > width {
					width = vdata[0].Width
				}
			}
			im := vaas.NewImage(width, height)
			canvas = &im

			stdout, cmd = vaas.MakeVideo(&vaas.ChanReader{ch}, width, height)
			donech = make(chan bool)
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

	err := vaas.ReadMultiple(r.slice.Length(), 1, flatInputs, vaas.ReadMultipleOptions{}, f)
	close(ch)
	if donech != nil {
		<- donech
	}

	if err != nil {
		r.setErr(err)
		log.Printf("[renderer] error rendering: %v", err)
		return
	}

	if r.opts.ProgressCallback != nil {
		r.opts.ProgressCallback(100)
	}

	r.mu.Lock()
	r.labels = labels
	r.done = true
	r.cond.Broadcast()
	r.mu.Unlock()
	log.Printf("[renderer] finished rendering video")
}

func (r *VideoRenderer) GetPreview() (vaas.Image, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for r.preview == nil && r.err == nil {
		r.cond.Wait()
	}
	if r.err != nil {
		return vaas.Image{}, r.err
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

func (r *VideoRenderer) GetLabels() ([][]vaas.Data, error) {
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

func (r *VideoRenderer) Wait() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for !r.done && r.err == nil {
		r.cond.Wait()
	}
	return r.err
}
