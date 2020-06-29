package skyhook

import (
	"log"
	"sync"
	"time"
)

type Suggestion struct {
	ID *int
	QueryID int
	Text string
	ActionLabel string
	Type string
	Config string
}

func (s *Suggestion) Save() {
	if s.ID == nil {
		res := db.Exec(
			"INSERT INTO suggestions (query_id, text, action_label, type, config) VALUES (?, ?, ?, ?, ?)",
			s.QueryID, s.Text, s.ActionLabel, s.Type, s.Config,
		)
		s.ID = new(int)
		*s.ID = res.LastInsertId()
	} else {
		db.Exec(
			"UPDATE suggestions SET query_id = ?, text = ?, action_label = ?, type = ?, config = ? WHERE id = ?",
			s.QueryID, s.Text, s.ActionLabel, s.Type, s.Config, s.ID,
		)
	}
}

const SuggestionQuery = "SELECT id, query_id, text, action_label, type, config FROM suggestions"

func suggestionListHelper(rows *Rows) []Suggestion {
	suggestions := []Suggestion{}
	for rows.Next() {
		var suggestion Suggestion
		rows.Scan(&suggestion.ID, &suggestion.QueryID, &suggestion.Text, &suggestion.ActionLabel, &suggestion.Type, &suggestion.Config)
		suggestions = append(suggestions, suggestion)
	}
	return suggestions
}

func ListQuerySuggestions(queryID int) []Suggestion {
	rows := db.Query(SuggestionQuery + " ORDER BY id DESC")
	return suggestionListHelper(rows)
}

func GetSuggestion(id int) *Suggestion {
	rows := db.Query(SuggestionQuery + " WHERE id = ?", id)
	suggestions := suggestionListHelper(rows)
	if len(suggestions) == 1 {
		suggestion := suggestions[0]
		return &suggestion
	} else {
		return nil
	}
}

type WatcherFunc func(map[int]StatsSample) *Suggestion

type Watcher struct {
	Get func(query *Query, suggestions []Suggestion) WatcherFunc
	Apply func(query *Query, suggestion Suggestion)
}

var Watchers = make(map[string]Watcher)

type WatchManager struct {
	mu sync.Mutex
	watcherFuncs map[int][]WatcherFunc
}

// Query was updated, reload the WatcherFuncs.
func (w *WatchManager) Reload(query *Query) {
	w.mu.Lock()
	defer w.mu.Unlock()
	existing := ListQuerySuggestions(query.ID)
	var funcs []WatcherFunc
	for _, watcher := range Watchers {
		f := watcher.Get(query, existing)
		if f != nil {
			funcs = append(funcs, f)
		}
	}
	w.watcherFuncs[query.ID] = funcs
}

// Background go-routine iteration.
func (w *WatchManager) iter() {
	for _, setID := range GetAllocator().GetEnvSets() {
		if setID.Type != "query" {
			continue
		}
		query := GetQuery(setID.RefID)
		query.Load()
		w.Reload(query)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	for queryID := range w.watcherFuncs {
		log.Printf("watching %d with %d funcs", queryID, len(w.watcherFuncs[queryID]))
		// collect stats
		containers := GetAllocator().GetContainers(EnvSetID{"query", queryID})
		if containers == nil {
			delete(w.watcherFuncs, queryID)
			continue
		}
		stats := GetAverageStatsByNode(containers)

		// get one suggestion
		var suggestion *Suggestion
		for _, f := range w.watcherFuncs[queryID] {
			suggestion = f(stats)
			if suggestion != nil {
				break
			}
		}
		if suggestion != nil {
			log.Printf("[watcher] adding new suggestion: %v", suggestion)
			suggestion.Save()
		}
	}
}

var watchman *WatchManager

func init() {
	watchman = &WatchManager{
		watcherFuncs: make(map[int][]WatcherFunc),
	}
	go func() {
		for {
			time.Sleep(5*time.Second)
			watchman.iter()
		}
	}()
}
