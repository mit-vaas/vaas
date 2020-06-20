package builtins

import (
	"../skyhook"
	"github.com/mitroadmaps/gomapinfer/common"
)

type Shape [][2]int

func (shp Shape) Bounds() common.Rectangle {
	r := common.EmptyRectangle
	for _, p := range shp {
		r = r.Extend(common.Point{float64(p[0]), float64(p[1])})
	}
	return r
}

func (shp Shape) Polygon() common.Polygon {
	poly := common.Polygon{}
	for _, p := range shp {
		poly = append(poly, common.Point{float64(p[0]), float64(p[1])})
	}
	return poly
}

func (shp Shape) Contains(d skyhook.Detection) bool {
	cx := (d.Left + d.Right)/2
	cy := (d.Top + d.Bottom)/2
	point := common.Point{float64(cx), float64(cy)}
	if len(shp) == 2 {
		return shp.Bounds().Contains(point)
	} else {
		return shp.Polygon().Contains(point)
	}
}

type TrackFilterConfig struct {
	// list of list of shapes
	// each shape is specified by a list of points
	// each row is a different alternative that can satisfy the predicate
	// within a row, the track must pass through all columns
	Shapes [][]Shape

	// whether the track must pass through columns in a row in order, or in any order
	Order bool
}

type TrackFilter struct {
	cfg TrackFilterConfig
}

func NewTrackFilter(node *skyhook.Node, query *skyhook.Query) skyhook.Executor {
	var cfg TrackFilterConfig
	skyhook.JsonUnmarshal([]byte(node.Code), &cfg)
	return TrackFilter{cfg: cfg}
}

func (m TrackFilter) Run(parents []skyhook.DataReader, slice skyhook.Slice) skyhook.DataBuffer {
	buf := skyhook.NewVideoBuffer(parents[0].Freq())

	go func() {
		data, err := parents[0].Read(slice.Length())
		if err != nil {
			buf.Error(err)
			return
		}
		parents[0].Close()
		detections := data.(skyhook.TrackData)
		tracks := skyhook.DetectionsToTracks(detections)
		var out [][]skyhook.DetectionWithFrame
		for _, track := range tracks {
			ok := false
			for _, alt := range m.cfg.Shapes {
				if m.cfg.Order {
					shapeIdx := 0
					for _, d := range track {
						if !alt[shapeIdx].Contains(d.Detection) {
							continue
						}
						shapeIdx++
						if shapeIdx >= len(alt) {
							break
						}
					}
					if shapeIdx >= len(alt) {
						ok = true
						break
					}
				} else {
					remainingShapes := make(map[int]bool)
					for i := range alt {
						remainingShapes[i] = true
					}
					for _, d := range track {
						for i := range remainingShapes {
							if !alt[i].Contains(d.Detection) {
								continue
							}
							delete(remainingShapes, i)
						}
					}
					if len(remainingShapes) == 0 {
						ok = true
						break
					}
				}
			}
			if ok {
				out = append(out, track)
			}
		}
		detections = skyhook.TracksToDetections(out)
		buf.Write(skyhook.TrackData(detections))
	}()

	return buf
}

func (m TrackFilter) Close() {}

func init() {
	skyhook.Executors["filter-track"] = NewTrackFilter
}
