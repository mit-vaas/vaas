package skyhook

import (
	"sync"
)

type TaskManager struct {
	activeQueries map[int]*QueryExecutor
	mu sync.Mutex
}

var tasks = &TaskManager{
	activeQueries: make(map[int]*QueryExecutor),
}

type Task struct {
	Query *Query
	Slices []ClipSlice
}

func (m *TaskManager) Schedule(task Task) []*BufferReader {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeQueries[task.Query.ID] == nil {
		for id, e := range m.activeQueries {
			e.Wait()
			delete(m.activeQueries, id)
		}
		m.activeQueries[task.Query.ID] = NewQueryExecutor(task.Query)
	}
	var outputs []*BufferReader
	for _, slice := range task.Slices {
		outputs = append(outputs, m.activeQueries[task.Query.ID].Run(nil, slice))
	}
	return outputs
}

func (m *TaskManager) nodeUpdated(node *Node) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, e := range m.activeQueries {
		if e.query.Nodes[node.ID] == nil {
			continue
		}
		e.Wait()
		delete(m.activeQueries, id)
	}
}
