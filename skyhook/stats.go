package skyhook

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
	var sum StatsSample
	for _, sample := range samples {
		sum = sum.Add(sample)
	}
	return StatsSample{
		Time: sum.Time / time.Duration(len(samples)),
	}
}

type StatsProvider interface {
	Stats() StatsSample
}
