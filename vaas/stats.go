package vaas

import (
	"sync"
	"time"
)

type StatsSample struct {
	Time struct {
		T time.Duration
		Count int
	}
	Idle struct {
		Fraction float64
		Count int
	}
}

func (s StatsSample) Add(other StatsSample) StatsSample {
	sum := StatsSample{}
	if s.Time.Count > 0 || other.Time.Count > 0 {
		count := s.Time.Count + other.Time.Count
		sum.Time.Count = count
		sum.Time.T = (s.Time.T*time.Duration(s.Time.Count) + other.Time.T*time.Duration(other.Time.Count)) / time.Duration(count)
	}
	if s.Idle.Count > 0 || other.Idle.Fraction > 0 {
		count := s.Idle.Count + other.Idle.Count
		sum.Idle.Count = count
		sum.Idle.Fraction = (s.Idle.Fraction*float64(s.Idle.Count) + other.Idle.Fraction*float64(other.Idle.Count)) / float64(count)
	}
	return sum
}

type StatsHolder struct {
	last StatsSample
	stats StatsSample
	mu sync.Mutex
	t time.Time
}

func (h *StatsHolder) Add(sample StatsSample) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if time.Now().Sub(h.t) > 30*time.Second {
		h.last = h.stats
		h.stats = sample
		h.t = time.Now()
		return
	}
	h.stats = h.stats.Add(sample)
}

func (h *StatsHolder) Get() StatsSample {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.last.Add(h.stats)
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
