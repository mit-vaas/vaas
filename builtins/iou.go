package builtins

// Simple overlap-based multi-object tracker.

import (
	"../vaas"
	goslgraph "github.com/cpmech/gosl/graph"

	"encoding/json"
	"fmt"
)

type TrackWithID struct {
	ID int
	Detections []vaas.Detection
	LastFrame int
}
func (t TrackWithID) Last() vaas.Detection {
	return t.Detections[len(t.Detections)-1]
}

type IOU struct {
	node vaas.Node
	maxAge int
	stats *vaas.StatsHolder
}

func NewIOU(node vaas.Node) vaas.Executor {
	type Config struct {
		MaxAge int `json:"maxAge"`
	}
	var cfg Config
	err := json.Unmarshal([]byte(node.Code), &cfg)
	if err != nil {
		cfg = Config{10}
	}
	return IOU{
		node: node,
		maxAge: cfg.MaxAge,
		stats: new(vaas.StatsHolder),
	}
}

func (m IOU) Run(ctx vaas.ExecContext) vaas.DataBuffer {
	parents, err := GetParents(ctx, m.node)
	if err != nil {
		return vaas.GetErrorBuffer(m.node.DataType, fmt.Errorf("iou error reading parents: %v", err))
	}
	buf := vaas.NewSimpleBuffer(vaas.TrackType)

	go func() {
		buf.SetMeta(parents[0].Freq())

		var nextID int = 1
		activeTracks := make(map[int]*TrackWithID)
		PerFrame(
			parents, ctx.Slice, buf, vaas.DetectionType,
			vaas.ReadMultipleOptions{Stats: m.stats},
			func(idx int, data vaas.Data, buf vaas.DataWriter) error {
				df := data.(vaas.DetectionData).D[0]
				detections := df.Detections
				var out []vaas.Detection

				matches := hungarianMatcher(activeTracks, detections)
				unmatched := make(map[int]vaas.Detection)
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
						Detections: []vaas.Detection{detection},
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

				ndata := vaas.DetectionData{
					T: vaas.TrackType,
					D: []vaas.DetectionFrame{{
						Detections: out,
						CanvasDims: df.CanvasDims,
					}},
				}
				buf.Write(ndata)
				return nil
			},
		)
	}()

	return buf
}

func (m IOU) Close() {}

// Returns map from track idx to detection idx that should be added corresponding to that track.
func hungarianMatcher(activeTracks map[int]*TrackWithID, detections []vaas.Detection) map[int]int {
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
		trackRect := vaas.DetectionToRectangle(track.Last())

		for j, detection := range detections {
			curRect := vaas.DetectionToRectangle(detection)
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

func (m IOU) Stats() vaas.StatsSample {
	return m.stats.Get()
}

func init() {
	vaas.Executors["iou"] = vaas.ExecutorMeta{New: NewIOU}
}
