package builtins

// Simple overlap-based multi-object tracker.

import (
	"../skyhook"
	goslgraph "github.com/cpmech/gosl/graph"

	"fmt"
)

type TrackWithID struct {
	ID int
	Detections []skyhook.Detection
	LastFrame int
}
func (t TrackWithID) Last() skyhook.Detection {
	return t.Detections[len(t.Detections)-1]
}

type IOU struct {
	node *skyhook.Node
	maxAge int
}

func NewIOU(node *skyhook.Node) skyhook.Executor {
	type Config struct {
		MaxAge int `json:"maxAge"`
	}
	var cfg Config
	skyhook.JsonUnmarshal([]byte(node.Code), &cfg)
	return IOU{
		node: node,
		maxAge: cfg.MaxAge,
	}
}

func (m IOU) Run(ctx skyhook.ExecContext) skyhook.DataBuffer {
	parents, err := GetParents(ctx, m.node)
	if err != nil {
		return skyhook.GetErrorBuffer(m.node.DataType, fmt.Errorf("iou error reading parents: %v", err))
	}
	buf := skyhook.NewSimpleBuffer(skyhook.TrackType)

	go func() {
		buf.SetMeta(parents[0].Freq())

		var nextID int = 1
		activeTracks := make(map[int]*TrackWithID)
		PerFrame(parents, ctx.Slice, buf, skyhook.DetectionType, func(idx int, data skyhook.Data, buf skyhook.DataWriter) error {
			detections := data.(skyhook.DetectionData)[0]
			var out []skyhook.Detection

			matches := hungarianMatcher(activeTracks, detections)
			unmatched := make(map[int]skyhook.Detection)
			for detectionIdx := range detections {
				unmatched[detectionIdx] = detections[detectionIdx]
			}
			for trackID, detectionIdx := range matches {
				delete(unmatched, detectionIdx)
				detection := detections[detectionIdx]
				detection.TrackID = trackID
				activeTracks[trackID].Detections = append(activeTracks[trackID].Detections, detection)
				activeTracks[trackID].LastFrame = idx
				out = append(out, detection)
			}
			for _, detection := range unmatched {
				trackID := nextID
				nextID++
				detection.TrackID = trackID
				track := &TrackWithID{
					ID: trackID,
					Detections: []skyhook.Detection{detection},
					LastFrame: idx,
				}
				activeTracks[track.ID] = track
				out = append(out, detection)
			}
			for _, track := range activeTracks {
				// TODO: make 10 a maxAge configurable parameter
				if idx - track.LastFrame < 10 {
					continue
				}
				delete(activeTracks, track.ID)
			}

			buf.Write(skyhook.TrackData{out})
			return nil
		})
	}()

	return buf
}

func (m IOU) Close() {}

// Returns map from track idx to detection idx that should be added corresponding to that track.
func hungarianMatcher(activeTracks map[int]*TrackWithID, detections []skyhook.Detection) map[int]int {
	if len(activeTracks) == 0 || len(detections) == 0 {
		return nil
	}

	var trackList []*TrackWithID
	for _, track := range activeTracks {
		trackList = append(trackList, track)
	}

	// create cost matrix for hungarian algorithm
	// rows: active tracks (trackList)
	// cols: current detections (detections)
	// values: 1-IoU if overlap is non-zero, or 1.5 otherwise
	costMatrix := make([][]float64, len(trackList))
	for i, track := range trackList {
		costMatrix[i] = make([]float64, len(detections))
		trackRect := skyhook.DetectionToRectangle(track.Last())

		for j, detection := range detections {
			curRect := skyhook.DetectionToRectangle(detection)
			iou := trackRect.IOU(curRect)
			var cost float64
			if iou > 0.99 {
				cost = 0.01
			} else if iou > 0.1 {
				cost = 1 - iou
			} else {
				cost = 1.5
			}
			costMatrix[i][j] = cost
		}
	}

	munkres := &goslgraph.Munkres{}
	munkres.Init(len(trackList), len(detections))
	munkres.SetCostMatrix(costMatrix)
	munkres.Run()

	matches := make(map[int]int)
	for i, j := range munkres.Links {
		track := trackList[i]
		if j < 0 || costMatrix[i][j] > 0.9 {
			continue
		}
		matches[track.ID] = j
	}
	return matches
}

func init() {
	skyhook.Executors["iou"] = NewIOU
}
