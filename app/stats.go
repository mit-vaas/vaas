package app

import (
	"../vaas"

	"log"
	"net/http"
	"sync"
	"time"
)

// container UUID -> node ID -> sample
type QueryStats map[string]map[int]vaas.StatsSample

type StatsManager struct {
	mu sync.Mutex
	stats map[int]QueryStats
}

func (m *StatsManager) OnQueryChanged(query *DBQuery) {
	m.mu.Lock()
	delete(m.stats, query.ID)
	m.mu.Unlock()
}

func (m *StatsManager) iter() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, setID := range GetAllocator().GetEnvSets() {
		if setID.Type != "query" {
			continue
		}
		queryID := setID.RefID
		if m.stats[queryID] == nil {
			m.stats[queryID] = make(QueryStats)
		}

		// collect stats
		containers := GetAllocator().GetContainers(setID)
		if containers == nil {
			continue
		}
		for _, clist := range containers {
			for _, container := range clist {
				stats := make(map[int]vaas.StatsSample)
				err := vaas.JsonPost(container.BaseURL, "/allstats", nil, &stats)
				if err != nil {
					log.Printf("[stats] warning: error reading stats from container %s (%s): %v", container.BaseURL, container.UUID, err)
					continue
				}
				m.stats[queryID][container.UUID] = stats
			}
		}
	}
}

func (m *StatsManager) GetStatsByNode(queryID int) map[int]vaas.StatsSample {
	m.mu.Lock()
	defer m.mu.Unlock()
	stats := make(map[int]vaas.StatsSample)
	for _, containerStats := range m.stats[queryID] {
		for nodeID, sample := range containerStats {
			stats[nodeID] = stats[nodeID].Add(sample)
		}
	}
	return stats
}

var statsManager *StatsManager

func init() {
	statsManager = &StatsManager{
		stats: make(map[int]QueryStats),
	}
	go func() {
		for {
			time.Sleep(5*time.Second)
			statsManager.iter()
		}
	}()

	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		queryID := vaas.ParseInt(r.Form.Get("query_id"))
		vaas.JsonResponse(w, statsManager.GetStatsByNode(queryID))
	})
}
