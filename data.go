package main

import (
	"fmt"
	"io"
	"sync"
)

type DataType string
const (
	DetectionType DataType = "detection"
	TrackType = "track"
	ClassType = "class"
	VideoType = "video"
)

type Detection struct {
	Left int `json:"left"`
	Top int `json:"top"`
	Right int `json:"right"`
	Bottom int `json:"bottom"`
	Score float64 `json:"score"`
	TrackID int `json:"track_id"`
}

func DetectionsToTracks(detections [][]Detection) [][]Detection {
	tracks := make(map[int][]Detection)
	for frameIdx := range detections {
		for _, detection := range detections[frameIdx] {
			if detection.TrackID < 0 {
				continue
			}
			tracks[detection.TrackID] = append(tracks[detection.TrackID], detection)
		}
	}
	var trackList [][]Detection
	for _, track := range tracks {
		trackList = append(trackList, track)
	}
	return trackList
}

type Data struct {
	Type DataType
	Detections [][]Detection
	Classes []int
	Images []Image
}

func (d Data) Nil() bool {
	return d.Type == ""
}

func (d Data) Length() int {
	if d.Type == DetectionType || d.Type == TrackType {
		return len(d.Detections)
	} else if d.Type == ClassType {
		return len(d.Classes)
	} else if d.Type == VideoType {
		return len(d.Images)
	}
	panic(fmt.Errorf("bad type %v", d.Type))
}

func (d Data) EnsureLength(length int) Data {
	if d.Type == DetectionType || d.Type == TrackType {
		for i := len(d.Detections); i < length; i++ {
			d.Detections = append(d.Detections, nil)
		}
	} else if d.Type == ClassType {
		for i := len(d.Classes); i < length; i++ {
			d.Classes = append(d.Classes, 0)
		}
	}
	return d
}

func (d Data) Slice(i, j int) Data {
	if d.Type == DetectionType || d.Type == TrackType {
		return Data{
			Type: d.Type,
			Detections: d.Detections[i:j],
		}
	} else if d.Type == ClassType {
		return Data{
			Type: d.Type,
			Classes: d.Classes[i:j],
		}
	} else if d.Type == VideoType {
		return Data{
			Type: d.Type,
			Images: d.Images[i:j],
		}
	}
	panic(fmt.Errorf("bad type %v", d.Type))
}

func (d Data) Append(other Data) Data {
	if d.Type == DetectionType || d.Type == TrackType {
		return Data{
			Type: d.Type,
			Detections: append(d.Detections, other.Detections...),
		}
	} else if d.Type == ClassType {
		return Data{
			Type: d.Type,
			Classes: append(d.Classes, other.Classes...),
		}
	} else if d.Type == VideoType {
		return Data{
			Type: d.Type,
			Images: append(d.Images, other.Images...),
		}
	}
	panic(fmt.Errorf("bad type %v", d.Type))
}

func (d Data) Get() interface{} {
	if d.Type == DetectionType || d.Type == TrackType {
		return d.Detections
	} else if d.Type == ClassType {
		return d.Classes
	} else if d.Type == VideoType {
		return d.Images
	}
	panic(fmt.Errorf("bad type %v", d.Type))
}

func (d Data) IsEmpty() bool {
	if d.Type == DetectionType || d.Type == TrackType {
		for _, dlist := range d.Detections {
			if len(dlist) > 0 {
				return false
			}
		}
		return true
	} else if d.Type == ClassType {
		for _, class := range d.Classes {
			if class > 0 {
				return false
			}
		}
		return true
	} else if d.Type == VideoType {
		return false
	}
	panic(fmt.Errorf("bad type %v", d.Type))
}

type LabelBuffer struct {
	unread Data
	mu sync.Mutex
	cond *sync.Cond
	err error
	done bool
}

func NewLabelBuffer(t DataType) *LabelBuffer {
	buf := &LabelBuffer{unread: Data{Type: t}}
	buf.cond = sync.NewCond(&buf.mu)
	return buf
}

// Read exactly n frames, or any non-zero amount if n<=0
func (buf *LabelBuffer) Read(n int) (Data, error) {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	for (buf.unread.Length() == 0 || buf.unread.Length() < n) && buf.err == nil && !buf.done {
		buf.cond.Wait()
	}
	if buf.err != nil {
		return Data{}, buf.err
	} else if buf.unread.Length() == 0 && buf.done {
		return Data{}, io.EOF
	} else if buf.unread.Length() < n {
		return Data{}, io.ErrUnexpectedEOF
	}

	var data Data
	if n <= 0 {
		data = buf.unread
		l := buf.unread.Length()
		buf.unread = buf.unread.Slice(l, l)
	} else {
		data = buf.unread.Slice(0, n)
		buf.unread = buf.unread.Slice(n, buf.unread.Length())
	}
	return data, nil
}

// Wait for at least n to complete, and read them without removing from the buffer.
func (buf *LabelBuffer) Peek(n int) (Data, error) {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	if n > 0 {
		for buf.unread.Length() < n && buf.err == nil && !buf.done {
			buf.cond.Wait()
		}
	}
	if buf.err != nil {
		return Data{}, buf.err
	}
	return buf.unread, nil
}

func (buf *LabelBuffer) Write(data Data) {
	buf.mu.Lock()
	buf.unread = buf.unread.Append(data)
	buf.cond.Broadcast()
	buf.mu.Unlock()
}

func (buf *LabelBuffer) Close() {
	buf.mu.Lock()
	buf.done = true
	buf.cond.Broadcast()
	buf.mu.Unlock()
}

func (buf *LabelBuffer) Error(err error) {
	buf.mu.Lock()
	buf.err = err
	buf.cond.Broadcast()
	buf.mu.Unlock()
}

func (buf *LabelBuffer) ReadAll(slice ClipSlice) (*LabeledClip, error) {
	data := Data{Type: buf.unread.Type}
	for data.Length() < slice.Length() {
		cur, err := buf.Read(0)
		if err != nil {
			return nil, err
		}
		data = data.Append(cur)
	}

	clipLabel := LabeledClip{
		Slice: slice,
		Type: data.Type,
		Label: data,
	}
	return &clipLabel, nil
}

func (buf *LabelBuffer) FromVideoReader(rd VideoReader) {
	for {
		im, err := rd.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			panic(err)
		}
		buf.Write(Data{
			Type: VideoType,
			Images: []Image{im},
		})
	}
}

// Returns a new buffer that reads from this buffer without removing the read
// content. This allows single-writer multi-reader mode, where all the readers
// read from different splits of the original buffer.
// This also optionally slices the buffer so that it only yields data in [start, end).
func (buf *LabelBuffer) Slice(start int, end int) *LabelBuffer {
	nbuf := NewLabelBuffer(buf.unread.Type)
	go func() {
		cur := start
		iter := func() bool {
			buf.mu.Lock()
			defer buf.mu.Unlock()
			for buf.unread.Length() <= cur && buf.err == nil && !buf.done {
				buf.cond.Wait()
			}
			if buf.err != nil {
				nbuf.Error(fmt.Errorf("passing error to downstream split buffer: %v", buf.err))
				return true
			} else if buf.unread.Length() <= cur && buf.done {
				nbuf.mu.Lock()
				nbuf.done = true
				nbuf.mu.Unlock()
				return true
			}

		}
		for cur < end {
			quit := iter()
			if quit {
				break
			}
		}
	}()
	return nbuf
}
