package vaas

import (
	gomapinfer "github.com/mitroadmaps/gomapinfer/common"
	"encoding/json"
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

func DetectionsToTracks(detections []DetectionFrame) [][]DetectionWithFrame {
	tracks := make(map[int][]DetectionWithFrame)
	for frameIdx, df := range detections {
		for _, detection := range df.Detections {
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

type DetectionFrame struct {
	Detections []Detection
	// may or may not be set
	CanvasDims [2]int `json:",omitempty"`
}

type DetectionData struct {
	T DataType `json:"-"`
	D []DetectionFrame
}
func (d DetectionData) IsEmpty() bool {
	for _, df := range d.D {
		if len(df.Detections) > 0 {
			return false
		}
	}
	return true
}
func (d DetectionData) Length() int {
	return len(d.D)
}
func (d DetectionData) EnsureLength(length int) Data {
	for len(d.D) < length {
		d.D = append(d.D, DetectionFrame{})
	}
	return d
}
func (d DetectionData) Slice(i, j int) Data {
	return DetectionData{
		T: d.T,
		D: d.D[i:j],
	}
}
func (d DetectionData) Append(other Data) Data {
	return DetectionData{
		T: d.T,
		D: append(d.D, other.(DetectionData).D...),
	}
}
func (d DetectionData) Encode() []byte {
	return JsonMarshal(d)
}
func (d DetectionData) Type() DataType {
	return d.T
}

func (d DetectionData) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.D)
}

func (d DetectionData) Resize(targetDims [2]int) DetectionData {
	out := DetectionData{T: d.T}
	for _, df := range d.D {
		var cur []Detection
		for _, detection := range df.Detections {
			if df.CanvasDims[0] != 0 && df.CanvasDims[1] != 0 {
				detection.Left = detection.Left * targetDims[0] / df.CanvasDims[0]
				detection.Right = detection.Right * targetDims[0] / df.CanvasDims[0]
				detection.Top = detection.Top * targetDims[1] / df.CanvasDims[1]
				detection.Bottom = detection.Bottom * targetDims[1] / df.CanvasDims[1]
			}
			cur = append(cur, detection)
		}
		out.D = append(out.D, DetectionFrame{
			Detections: cur,
			CanvasDims: targetDims,
		})
	}
	return out
}

func init() {
	dataImpls[DetectionType] = DataImpl{
		New: func() Data {
			return DetectionData{T: DetectionType}
		},
		Decode: func(bytes []byte) Data {
			d := DetectionData{T: DetectionType}
			JsonUnmarshal(bytes, &d.D)
			return d
		},
	}
	dataImpls[TrackType] = DataImpl{
		New: func() Data {
			return DetectionData{T: TrackType}
		},
		Decode: func(bytes []byte) Data {
			d := DetectionData{T: TrackType}
			JsonUnmarshal(bytes, &d.D)
			return d
		},
	}
}
