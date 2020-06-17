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
	Vector []*Series
	Slices []Slice
}

func (m *TaskManager) Schedule(task Task) [][][]DataReader {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeQueries[task.Query.ID] == nil {
		for id, e := range m.activeQueries {
			e.Wait()
			delete(m.activeQueries, id)
		}
		m.activeQueries[task.Query.ID] = NewQueryExecutor(task.Query)
	}
	donech := make(chan bool)
	taskOutputs := make([][][]DataReader, len(task.Slices))
	for i, slice := range task.Slices {
		idx := i
		m.activeQueries[task.Query.ID].Run(task.Vector, slice, func(outputs [][]DataReader, err error) {
			taskOutputs[idx] = outputs
			donech <- true
		})
	}
	for _ = range task.Slices {
		<- donech
	}
	return taskOutputs
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
