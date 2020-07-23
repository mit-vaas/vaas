package builtins

import (
	"../vaas"
	"github.com/mitroadmaps/gomapinfer/common"
	"fmt"
	"time"
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

func (shp Shape) Contains(d vaas.Detection) bool {
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
	ScaleX float64
	ScaleY float64

	// list of list of shapes
	// each shape is specified by a list of points
	// each row is a different alternative that can satisfy the predicate
	// within a row, the track must pass through all columns
	Shapes [][]Shape

	// whether the track must pass through columns in a row in order, or in any order
	Order bool
}

type TrackFilter struct {
	node vaas.Node
	cfg TrackFilterConfig
	stats *vaas.StatsHolder
}

func NewTrackFilter(node vaas.Node) vaas.Executor {
	var cfg TrackFilterConfig
	vaas.JsonUnmarshal([]byte(node.Code), &cfg)
	for i := range cfg.Shapes {
		for j := range cfg.Shapes[i] {
			for k, p := range cfg.Shapes[i][j] {
				cfg.Shapes[i][j][k] = [2]int{
					int(float64(p[0])*cfg.ScaleX),
					int(float64(p[1])*cfg.ScaleY),
				}
			}
		}
	}
	return TrackFilter{
		node: node,
		cfg: cfg,
		stats: new(vaas.StatsHolder),
	}
}

func (m TrackFilter) Run(ctx vaas.ExecContext) vaas.DataBuffer {
	parents, err := GetParents(ctx, m.node)
	if err != nil {
		return vaas.GetErrorBuffer(m.node.DataType, fmt.Errorf("track-filter error reading parents: %v", err))
	}
	buf := vaas.NewSimpleBuffer(vaas.TrackType)

	go func() {
		t0 := time.Now()
		buf.SetMeta(parents[0].Freq())
		data, err := parents[0].Read(ctx.Slice.Length())
		if err != nil {
			buf.Error(fmt.Errorf("filter_track error reading parent: %v", err))
			return
		}
		parents[0].Close()

		t1 := time.Now()
		detections := data.(vaas.TrackData)
		tracks := vaas.DetectionsToTracks(detections)
		var out [][]vaas.DetectionWithFrame
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
		detections = vaas.TracksToDetections(out)
		buf.Write(vaas.TrackData(detections).EnsureLength(data.Length()))
		buf.Close()

		t2 := time.Now()
		sample := vaas.StatsSample{}
		sample.Idle.Fraction = float64(t1.Sub(t0)) / float64(t2.Sub(t0))
		sample.Idle.Count = ctx.Slice.Length()
		m.stats.Add(sample)
	}()

	return buf
}

func (m TrackFilter) Close() {}

func (m TrackFilter) Stats() vaas.StatsSample {
	return m.stats.Get()
}

func init() {
	vaas.Executors["filter-track"] = vaas.ExecutorMeta{New: NewTrackFilter}
}
