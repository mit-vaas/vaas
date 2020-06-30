package vaas

import (
	"fmt"
	"io"
	"sync"
)

type jpegReader struct {
	item Item
	start int
	end int
	pos int
}

func ReadJpeg(item Item, start int, end int, opts ReadVideoOptions) *jpegReader {
	if opts.Scale[0] != 0 || opts.Sample != 1 {
		panic(fmt.Errorf("jpegReader does not yet support rescale/resample"))
	}
	return &jpegReader{
		item: item,
		start: start,
		end: end,
		pos: start,
	}
}

func (rd *jpegReader) Read() (Image, error) {
	if rd.pos >= rd.end {
		return Image{}, io.EOF
	}
	im := ImageFromFile(rd.item.Fname(rd.pos))
	rd.pos++
	return im, nil
}
func (rd *jpegReader) Close() {}

type parallelJpegReader struct {
	item Item
	start int
	end int
	pos int

	mu *sync.Mutex
	cond *sync.Cond
	buffer map[int]*Image
	next int
}

func ReadJpegParallel(item Item, start int, end int, nthreads int, opts ReadVideoOptions) *parallelJpegReader {
	if opts.Scale[0] != 0 || opts.Sample != 1 {
		panic(fmt.Errorf("jpegReader does not yet support rescale/resample"))
	}
	rd := &parallelJpegReader{
		item: item,
		start: start,
		end: end,
		pos: start,
		buffer: make(map[int]*Image),
		next: start,
	}
	rd.mu = new(sync.Mutex)
	rd.cond = sync.NewCond(rd.mu)

	for i := 0; i < nthreads; i++ {
		go func() {
			for {
				rd.mu.Lock()
				next := rd.next
				if next == rd.end {
					rd.mu.Unlock()
					break
				}
				rd.next++
				rd.mu.Unlock()
				im := ImageFromFile(rd.item.Fname(next))
				rd.mu.Lock()
				rd.buffer[next] = &im
				rd.cond.Broadcast()
				rd.mu.Unlock()
			}
		}()
	}

	return rd
}

func (rd *parallelJpegReader) Read() (Image, error) {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	if rd.pos >= rd.end {
		return Image{}, io.EOF
	}
	for rd.buffer[rd.pos] == nil {
		rd.cond.Wait()
	}
	im := rd.buffer[rd.pos]
	delete(rd.buffer, rd.pos)
	rd.pos++
	return *im, nil
}

// TODO: maybe we should limit the buffer size
func (rd *parallelJpegReader) Close() {}
