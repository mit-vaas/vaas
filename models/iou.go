package models

// Simple overlap-based multi-object tracker.

import (
	"../skyhook"
	gomapinfer "github.com/mitroadmaps/gomapinfer/common"
	goslgraph "github.com/cpmech/gosl/graph"
)

func DetectionToRectangle(d skyhook.Detection) gomapinfer.Rectangle {
	return gomapinfer.Rectangle{
		gomapinfer.Point{float64(d.Left), float64(d.Top)},
		gomapinfer.Point{float64(d.Right), float64(d.Bottom)},
	}
}

type TrackWithID struct {
	ID int
	Detections []skyhook.Detection
	LastFrame int
}
func (t TrackWithID) Last() skyhook.Detection {
	return t.Detections[len(t.Detections)-1]
}

type IOU struct {}

func NewIOU(cfgBytes []byte) skyhook.Executor {
	return IOU{}
}

func (m IOU) Run(parents []*skyhook.BufferReader, slice skyhook.Slice) *skyhook.DataBuffer {
	buf := skyhook.NewDataBuffer(skyhook.TrackType)

	go func() {
		var nextID int = 1
		activeTracks := make(map[int]*TrackWithID)
		PerFrame(parents, slice, buf, skyhook.DetectionType, func(idx int, data skyhook.Data, buf *skyhook.DataBuffer) error {
			detections := data.Detections[0]

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
			}
			for _, track := range activeTracks {
				// TODO: make 10 a maxAge configurable parameter
				if idx - track.LastFrame < 10 {
					continue
				}
				delete(activeTracks, track.ID)
			}

			buf.Write(skyhook.Data{
				Type: skyhook.TrackType,
				Detections: [][]skyhook.Detection{detections},
			})
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
		trackRect := DetectionToRectangle(track.Last())

		for j, detection := range detections {
			curRect := DetectionToRectangle(detection)
			iou := trackRect.Min.X + curRect.Min.X
			iou = 1.0
			//iou := trackRect.IOU(curRect)
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
	skyhook.Models["iou"] = NewIOU
}