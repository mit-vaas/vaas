package vaas

import (
	gomapinfer "github.com/mitroadmaps/gomapinfer/common"
)

type Detection struct {
	Left int `json:"left"`
	Top int `json:"top"`
	Right int `json:"right"`
	Bottom int `json:"bottom"`
	Score float64 `json:"score,omitempty"`
	Class string `json:"class,omitempty"`
	TrackID int `json:"track_id"`
}

func DetectionToRectangle(d Detection) gomapinfer.Rectangle {
	return gomapinfer.Rectangle{
		gomapinfer.Point{float64(d.Left), float64(d.Top)},
		gomapinfer.Point{float64(d.Right), float64(d.Bottom)},
	}
}

type DetectionWithFrame struct {
	Detection
	FrameIdx int
}

func DetectionsToTracks(detections [][]Detection) [][]DetectionWithFrame {
	tracks := make(map[int][]DetectionWithFrame)
	for frameIdx := range detections {
		for _, detection := range detections[frameIdx] {
			if detection.TrackID < 0 {
				continue
			}
			tracks[detection.TrackID] = append(tracks[detection.TrackID], DetectionWithFrame{
				Detection: detection,
				FrameIdx: frameIdx,
			})
		}
	}
	var trackList [][]DetectionWithFrame
	for _, track := range tracks {
		trackList = append(trackList, track)
	}
	return trackList
}

func TracksToDetections(tracks [][]DetectionWithFrame) [][]Detection {
	var detections [][]Detection
	for _, track := range tracks {
		for _, d := range track {
			for d.FrameIdx >= len(detections) {
				detections = append(detections, []Detection{})
			}
			detections[d.FrameIdx] = append(detections[d.FrameIdx], d.Detection)
		}
	}
	return detections
}

func ResizeDetections(detections [][]Detection, scale [2]float64) [][]Detection {
	var out [][]Detection
	for frameIdx := range detections {
		var cur []Detection
		for _, detection := range detections[frameIdx] {
			detection.Left = int(float64(detection.Left) * scale[0])
			detection.Right = int(float64(detection.Right) * scale[0])
			detection.Top = int(float64(detection.Top) * scale[1])
			detection.Bottom = int(float64(detection.Bottom) * scale[1])
			cur = append(cur, detection)
		}
		out = append(out, cur)
	}
	return out
}

type DetectionData [][]Detection
func (d DetectionData) IsEmpty() bool {
	for _, dlist := range d {
		if len(dlist) > 0 {
			return false
		}
	}
	return true
}
func (d DetectionData) Length() int {
	return len(d)
}
func (d DetectionData) EnsureLength(length int) Data {
	for i := len(d); i < length; i++ {
		d = append(d, nil)
	}
	return d
}
func (d DetectionData) Slice(i, j int) Data {
	return d[i:j]
}
func (d DetectionData) Append(other Data) Data {
	other_ := other.(DetectionData)
	return append(d, other_...)
}
func (d DetectionData) Encode() []byte {
	return JsonMarshal(d)
}
func (d DetectionData) Type() DataType {
	return DetectionType
}

type TrackData [][]Detection
func (d TrackData) IsEmpty() bool {
	for _, dlist := range d {
		if len(dlist) > 0 {
			return false
		}
	}
	return true
}
func (d TrackData) Length() int {
	return len(d)
}
func (d TrackData) EnsureLength(length int) Data {
	for i := len(d); i < length; i++ {
		d = append(d, nil)
	}
	return d
}
func (d TrackData) Slice(i, j int) Data {
	return d[i:j]
}
func (d TrackData) Append(other Data) Data {
	other_ := other.(TrackData)
	return append(d, other_...)
}
func (d TrackData) Encode() []byte {
	return JsonMarshal(d)
}
func (d TrackData) Type() DataType {
	return TrackType
}

func init() {
	dataImpls[DetectionType] = DataImpl{
		New: func() Data {
			return DetectionData{}
		},
		Decode: func(bytes []byte) Data {
			var d DetectionData
			JsonUnmarshal(bytes, &d)
			return d
		},
	}
	dataImpls[TrackType] = DataImpl{
		New: func() Data {
			return TrackData{}
		},
		Decode: func(bytes []byte) Data {
			var d TrackData
			JsonUnmarshal(bytes, &d)
			return d
		},
	}
}
