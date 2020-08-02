package app

import (
	"../vaas"
	"log"
	"net/http"
	"sync"
	"time"
)

type Suggestion struct {
	QueryID int
	Text string
	ActionLabel string
	Type string
	Config string
}

type WatcherFunc func(map[int]vaas.StatsSample) *Suggestion

type Watcher struct {
	Get func(query *DBQuery, suggestions []Suggestion) WatcherFunc
	Apply func(query *DBQuery, suggestion Suggestion)
}

var Watchers = make(map[string]Watcher)

type WatchManager struct {
	mu sync.Mutex
	watcherFuncs map[int][]WatcherFunc
	suggestions map[int][]Suggestion
}

// Unload the WatcherFuncs whenever a query is modified.
func (w *WatchManager) OnQueryChanged(query *DBQuery) {
	w.mu.Lock()
	delete(w.suggestions, query.ID)
	delete(w.watcherFuncs, query.ID)
	w.mu.Unlock()
}

func (w *WatchManager) reload(query *DBQuery) {
	query.Load()
	var funcs []WatcherFunc
	for _, watcher := range Watchers {
		f := watcher.Get(query, w.suggestions[query.ID])
		if f != nil {
			funcs = append(funcs, f)
		}
	}
	log.Printf("[watcher] watching query %s with %d funcs", query.Name, len(w.watcherFuncs[query.ID]))
	w.watcherFuncs[query.ID] = funcs
}

// Background go-routine iteration.
func (w *WatchManager) iter() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, setID := range GetAllocator().GetEnvSets() {
		if setID.Type != "query" {
			continue
		}
		queryID := setID.RefID
		if w.watcherFuncs[queryID] == nil {
			w.reload(GetQuery(queryID))
		}
		stats := statsManager.GetStatsByNode(queryID)

		// get one suggestion
		var suggestion *Suggestion
		for _, f := range w.watcherFuncs[queryID] {
			suggestion = f(stats)
			if suggestion != nil {
				break
			}
		}
		if suggestion != nil {
			log.Printf("[watcher] adding new suggestion: %v", *suggestion)
			w.suggestions[queryID] = append(w.suggestions[queryID], *suggestion)
			w.reload(GetQuery(queryID))
		}
	}
}

func (w *WatchManager) ListSuggestions(queryID int) []Suggestion {
	w.mu.Lock()
	defer w.mu.Unlock()
	suggestions := w.suggestions[queryID]
	if suggestions == nil {
		// don't want JSON null
		return []Suggestion{}
	}
	return suggestions
}

var watchman *WatchManager

func init() {
	watchman = &WatchManager{
		watcherFuncs: make(map[int][]WatcherFunc),
		suggestions: make(map[int][]Suggestion),
	}
	go func() {
		for {
			time.Sleep(5*time.Second)
			watchman.iter()
		}
	}()

	http.HandleFunc("/suggestions", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		queryID := vaas.ParseInt(r.Form.Get("query_id"))
		vaas.JsonResponse(w, watchman.ListSuggestions(queryID))
	})

	http.HandleFunc("/suggestions/apply", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		var suggestion Suggestion
		if err := vaas.ParseJsonRequest(w, r, &suggestion); err != nil {
			return
		}
		query := GetQuery(suggestion.QueryID)
		query.Load()
		log.Printf("[watcher] applying suggestion [%s] (%s) for query %s", suggestion.Text, suggestion.Type, query.Name)
		Watchers[suggestion.Type].Apply(query, suggestion)
		allocator.Deallocate(vaas.EnvSetID{"query", query.ID})
	})
}
