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
}

func NewDetectionFilter(node vaas.Node) vaas.Executor {
	var cfg DetectionFilterConfig
	vaas.JsonUnmarshal([]byte(node.Code), &cfg)
	m := DetectionFilter{
		node: node,
		score: cfg.Score,
		classes: make(map[string]bool),
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
		PerFrame(parents, ctx.Slice, buf, vaas.DetectionType, func(idx int, data vaas.Data, buf vaas.DataWriter) error {
			detections := data.(vaas.DetectionData)[0]
			var ndetections []vaas.Detection
			for _, d := range detections {
				if d.Score < m.score {
					continue
				}
				if len(m.classes) > 0 && !m.classes[d.Class] {
					continue
				}
				ndetections = append(ndetections, d)
			}
			buf.Write(vaas.DetectionData{ndetections}.EnsureLength(data.Length()))
			return nil
		})
	}()

	return buf
}

func (m DetectionFilter) Close() {}

func init() {
	vaas.Executors["filter-detection"] = NewDetectionFilter
}
