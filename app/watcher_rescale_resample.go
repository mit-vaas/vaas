package app

import (
	"../vaas"
	"time"
)

func init() {
	Watchers["rescale-resample"] = Watcher{
		Get: func(query *DBQuery, suggestions []Suggestion) WatcherFunc {
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

			return func(stats map[int]vaas.StatsSample) *Suggestion {
				// TODO: should make sure input is high resolution or high sample rate
				// for now, find node that inputs video and outputs non-video, which is slow
				suggest := false
				for nodeID, sample := range stats {
					node := query.Nodes[nodeID]
					if node.DataType == vaas.VideoType {
						continue
					}
					if sample.Time < time.Second/time.Duration(vaas.FPS) {
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
		Apply: func(query *DBQuery, suggest Suggestion) {
			// for now just add a rescale/resample before the input
			rrNode := query.AddNode("Tunable Rescale/Resample", "rescale-resample", vaas.VideoType)
			rrNode.Parents = []vaas.Parent{{
				Type: vaas.SeriesParent,
				SeriesIdx: 0,
			}}
			rrNode.Save()

			for _, node := range query.Nodes {
				if node.ID == rrNode.ID {
					continue
				}
				updated := false
				for i, parent := range node.Parents {
					if parent.Type != vaas.SeriesParent || parent.SeriesIdx != 0 {
						continue
					}
					updated = true
					node.Parents[i] = vaas.Parent{
						Type: vaas.NodeParent,
						NodeID: rrNode.ID,
					}
				}
				if updated {
					DBNode{Node: *node}.Save()
				}
			}
		},
	}
}
