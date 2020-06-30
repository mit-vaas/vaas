package vaas

import (
	"time"
)

type StatsSample struct {
	Time time.Duration
}

func (s StatsSample) Add(other StatsSample) StatsSample {
	return StatsSample{
		Time: s.Time + other.Time,
	}
}

func AverageStats(samples []StatsSample) StatsSample {
	if len(samples) == 0 {
		return StatsSample{}
	}
	var sum StatsSample
	for _, sample := range samples {
		sum = sum.Add(sample)
	}
	return StatsSample{
		Time: sum.Time / time.Duration(len(samples)),
	}
}

type WeightedStats struct {
	Sample StatsSample
	Weight int
}

func (ws *WeightedStats) Add(sample StatsSample) {
	ws.Sample = StatsSample{
		ws.Sample.Time * time.Duration(ws.Weight) + sample.Time / (time.Duration(ws.Weight) + 1),
	}
	ws.Weight++
}

type StatsProvider interface {
	Stats() StatsSample
}

func GetStatsSamplesByNode(containers [][]Container) map[int][]StatsSample {
	samples := make(map[int][]StatsSample)
	for _, clist := range containers {
		for _, container := range clist {
			statsMap := make(map[int]StatsSample)
			err := JsonPost(container.BaseURL, "/allstats", nil, &statsMap)
			if err != nil {
				panic(err)
			}
			for nodeID, sample := range statsMap {
				samples[nodeID] = append(samples[nodeID], sample)
			}
		}
	}
	return samples
}

func GetAverageStatsByNode(containers [][]Container) map[int]StatsSample {
	stats := make(map[int]StatsSample)
	for nodeID, l := range GetStatsSamplesByNode(containers) {
		stats[nodeID] = AverageStats(l)
	}
	return stats
}
