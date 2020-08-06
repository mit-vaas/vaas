package builtins

import (
	"../vaas"
	"fmt"
)

type DetectionFilterConfig struct {
	Score float64
	Classes []string
}

type DetectionFilter struct {
	node vaas.Node
	score float64
	classes map[string]bool
	stats *vaas.StatsHolder
}

func NewDetectionFilter(node vaas.Node) vaas.Executor {
	var cfg DetectionFilterConfig
	vaas.JsonUnmarshal([]byte(node.Code), &cfg)
	m := DetectionFilter{
		node: node,
		score: cfg.Score,
		classes: make(map[string]bool),
		stats: new(vaas.StatsHolder),
	}
	for _, class := range cfg.Classes {
		m.classes[class] = true
	}
	return m
}

func (m DetectionFilter) Run(ctx vaas.ExecContext) vaas.DataBuffer {
	parents, err := GetParents(ctx, m.node)
	if err != nil {
		return vaas.GetErrorBuffer(m.node.DataType, fmt.Errorf("detection-filter error reading parents: %v", err))
	}
	buf := vaas.NewSimpleBuffer(vaas.DetectionType)

	go func() {
		buf.SetMeta(parents[0].Freq())
		PerFrame(
			parents, ctx.Slice, buf, vaas.DetectionType,
			vaas.ReadMultipleOptions{Stats: m.stats},
			func(idx int, data vaas.Data, buf vaas.DataWriter) error {
				df := data.(vaas.DetectionData).D[0]
				var ndetections []vaas.Detection
				for _, d := range df.Detections {
					if d.Score < m.score {
						continue
					}
					if len(m.classes) > 0 && !m.classes[d.Class] {
						continue
					}
					ndetections = append(ndetections, d)
				}
				ndata := vaas.DetectionData{
					T: vaas.DetectionType,
					D: []vaas.DetectionFrame{{
						Detections: ndetections,
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

func (m DetectionFilter) Close() {}

func (m DetectionFilter) Stats() vaas.StatsSample {
	return m.stats.Get()
}

func init() {
	vaas.Executors["filter-detection"] = vaas.ExecutorMeta{New: NewDetectionFilter}
}
