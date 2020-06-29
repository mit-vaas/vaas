package skyhook

import (
	"time"
)

func init() {
	Watchers["rescale-resample"] = Watcher{
		Get: func(query *Query, suggestions []Suggestion) WatcherFunc {
			// no func if already have rescale-resample suggestion
			for _, suggestion := range suggestions {
				if suggestion.Type == "rescale-resample" {
					return nil
				}
			}

			// must not have rescale-resample node
			for _, node := range query.Nodes {
				if node.Type == "rescale-resample" {
					return nil
				}
			}

			return func(stats map[int]StatsSample) *Suggestion {
				// TODO: should make sure input is high resolution or high sample rate
				// for now, find node that inputs video and outputs non-video, which is slow
				suggest := false
				for nodeID, sample := range stats {
					node := query.Nodes[nodeID]
					if node.DataType == VideoType {
						continue
					}
					if sample.Time < time.Second/time.Duration(FPS) {
						continue
					}
					suggest = true
					break
				}
				if !suggest {
					return nil
				}
				return &Suggestion{
					QueryID: query.ID,
					Text: "Rescale or resample inputs to reduce processing time.",
					ActionLabel: "Add a tunable Rescale-Resample node",
					Type: "rescale-resample",
					Config: "",
				}
			}
		},
		Apply: func(query *Query, suggest Suggestion) {
			// for now just add a rescale/resample before the input
			rrNode := query.AddNode("Tunable Rescale/Resample", "rescale-resample", VideoType)
			rrNode.Parents = []Parent{{
				Type: SeriesParent,
				SeriesIdx: 0,
			}}
			rrNode.Save()

			for _, node := range query.Nodes {
				if node == rrNode {
					continue
				}
				updated := false
				for i, parent := range node.Parents {
					if parent.Type != SeriesParent || parent.SeriesIdx != 0 {
						continue
					}
					updated = true
					node.Parents[i] = Parent{
						Type: NodeParent,
						NodeID: rrNode.ID,
					}
				}
				if updated {
					node.Save()
				}
			}
		},
	}
}
