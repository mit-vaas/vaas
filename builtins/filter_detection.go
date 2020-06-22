package builtins

import (
	"../skyhook"
)

type DetectionFilterConfig struct {
	Score float64
	Classes []string
}

type DetectionFilter struct {
	score float64
	classes map[string]bool
}

func NewDetectionFilter(node *skyhook.Node, query *skyhook.Query) skyhook.Executor {
	var cfg DetectionFilterConfig
	skyhook.JsonUnmarshal([]byte(node.Code), &cfg)
	m := DetectionFilter{
		score: cfg.Score,
		classes: make(map[string]bool),
	}
	for _, class := range cfg.Classes {
		m.classes[class] = true
	}
	return m
}

func (m DetectionFilter) Run(parents []skyhook.DataReader, slice skyhook.Slice) skyhook.DataBuffer {
	buf := skyhook.NewSimpleBuffer(skyhook.DetectionType, parents[0].Freq())

	go func() {
		PerFrame(parents, slice, buf, skyhook.DetectionType, func(idx int, data skyhook.Data, buf skyhook.DataWriter) error {
			detections := data.(skyhook.DetectionData)[0]
			var ndetections []skyhook.Detection
			for _, d := range detections {
				if d.Score < m.score {
					continue
				}
				if len(m.classes) > 0 && !m.classes[d.Class] {
					continue
				}
				ndetections = append(ndetections, d)
			}
			buf.Write(skyhook.DetectionData{ndetections}.EnsureLength(data.Length()))
			return nil
		})
	}()

	return buf
}

func (m DetectionFilter) Close() {}

func init() {
	skyhook.Executors["filter-detection"] = NewDetectionFilter
}
