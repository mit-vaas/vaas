package main

import (
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
	FrameIdx int `json:"frame_idx"`
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

func MergeLabels(labels []interface{}, t DataType) interface{} {
	var merged interface{}
	if t == DetectionType || t == TrackType {
		var detections [][]Detection
		for _, label := range labels {
			cur := label.([][]Detection)
			detections = append(detections, cur...)
		}
		merged = detections
	} else if t == ClassType {
		var classes []int
		for _, label := range labels {
			cur := label.([]int)
			classes = append(classes, cur...)
		}
		merged = classes
	} else if t == VideoType {
		var images []Image
		for _, label := range labels {
			cur := label.([]Image)
			images = append(images, cur...)
		}
		merged = images
	}
	return merged
}

func IsEmptyLabel(label interface{}, t DataType) bool {
	if t == DetectionType || t == TrackType {
		detections := label.([][]Detection)
		for _, dlist := range detections {
			if len(dlist) > 0 {
				return false
			}
		}
	}
	return true
}

type LabelPacket struct {
	Start int
	End int
	Data interface{}
}

type LabelBuffer struct {
	unread []LabelPacket
	mu *sync.Mutex
	cond *sync.Cond
	err error
	done bool
}

func NewLabelBuffer() *LabelBuffer {
	buf := &LabelBuffer{}
	buf.mu = new(sync.Mutex)
	buf.cond = sync.NewCond(buf.mu)
	return buf
}

func FilledLabelBuffer(n int, label interface{}) *LabelBuffer {
	buf := &LabelBuffer{
		done: true,
		unread: []LabelPacket{{
			Start: 0,
			End: n,
			Data: label,
		}},
	}
	buf.mu = new(sync.Mutex)
	buf.cond = sync.NewCond(buf.mu)
	return buf
}

func (buf *LabelBuffer) Read() (int, interface{}, error) {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	for len(buf.unread) == 0 && buf.err == nil && !buf.done {
		buf.cond.Wait()
	}
	if buf.err != nil {
		return 0, nil, buf.err
	} else if len(buf.unread) == 0 && buf.done {
		return 0, nil, io.EOF
	}
	packet := buf.unread[0]
	buf.unread = buf.unread[0:copy(buf.unread[0:], buf.unread[1:])]
	return packet.End-packet.Start, packet.Data, nil
}

// Wait for at least n to complete, and read them without removing from the buffer.
func (buf *LabelBuffer) Peek(n int, t DataType) (int, interface{}, error) {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	if n > 0 {
		for (len(buf.unread) == 0 || buf.unread[len(buf.unread)-1].End < n) && buf.err == nil && !buf.done {
			buf.cond.Wait()
		}
	}
	if buf.err != nil {
		return 0, nil, buf.err
	}
	var labels []interface{}
	total := 0
	for _, packet := range buf.unread {
		total += packet.End - packet.Start
		labels = append(labels, packet.Data)
	}
	return total, MergeLabels(labels, t), nil
}

func (buf *LabelBuffer) Write(start int, end int, data interface{}) {
	buf.mu.Lock()
	buf.unread = append(buf.unread, LabelPacket{start, end, data})
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

func (buf *LabelBuffer) ReadAll(slice ClipSlice, t DataType) (*LabeledClip, error) {
	var labels []interface{}
	total := 0
	for total < slice.Length() {
		n, label, err := buf.Read()
		if err != nil {
			return nil, err
		}
		total += n
		labels = append(labels, label)
	}

	// merge labels
	clipLabel := LabeledClip{
		Slice: slice,
		Type: t,
		Label: MergeLabels(labels, t),
	}
	return &clipLabel, nil
}
