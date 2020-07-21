package vaas

import (
	"time"
)

type StatsSample struct {
	Time time.Duration
	Count int
}

func (s StatsSample) Add(other StatsSample) StatsSample {
	count := s.Count + other.Count
	if count == 0 {
		return s
	}
	return StatsSample{
		Time: (s.Time*time.Duration(s.Count) + other.Time*time.Duration(other.Count)) / time.Duration(count),
		Count: count,
	}
}

type StatsProvider interface {
	Stats() StatsSample
}

func GetStatsByNode(containers [][]Container) map[int]StatsSample {
	stats := make(map[int]StatsSample)
	for _, clist := range containers {
		for _, container := range clist {
			statsMap := make(map[int]StatsSample)
			err := JsonPost(container.BaseURL, "/allstats", nil, &statsMap)
			if err != nil {
				panic(err)
			}
			for nodeID, sample := range statsMap {
				stats[nodeID] = stats[nodeID].Add(sample)
			}
		}
	}
	return stats
}
