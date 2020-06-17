package skyhook

import (
	"fmt"
	"log"
	"strings"
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
	Callback func(Slice, [][]DataReader, error)
}

func (m *TaskManager) Schedule(task Task) {
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
	for _, slice := range task.Slices {
		slice_ := slice
		m.activeQueries[task.Query.ID].Run(task.Vector, slice, func(outputs [][]DataReader, err error) {
			task.Callback(slice_, outputs, err)
			donech <- true
		})
	}
	for _ = range task.Slices {
		<- donech
	}
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

type extraOutput struct {
	slice Slice
	outputs [][]DataReader
}

type TaskContext struct {
	query *Query
	vector []*Series
	sampler func() *Slice
	perIter int
	callback func(Slice, [][]DataReader, error)
	seenSegments map[string]bool
	mu sync.Mutex

	running bool
	closed bool

	remaining int

	// extra outputs we haven't sent to callback
	extras []extraOutput
}

func NewTaskContext(query *Query, vector []*Series, sampler func() *Slice, perIter int, callback func(Slice, [][]DataReader, error)) *TaskContext {
	return &TaskContext{
		query: query,
		vector: vector,
		sampler: sampler,
		perIter: perIter,
		seenSegments: make(map[string]bool),
		callback: callback,
	}
}

func (ctx *TaskContext) Get(n int) {
	// called while we have lock
	setErr := func(err error) {
		for i := 0; i < ctx.remaining; i++ {
			ctx.callback(Slice{}, nil, err)
		}
		ctx.remaining = 0
	}

	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.closed {
		setErr(fmt.Errorf("task context is closed"))
		return
	} else if ctx.running {
		ctx.remaining += n
		return
	}
	ctx.remaining = n
	ctx.running = true

	go func() {
		ctx.mu.Lock()
		for !ctx.closed && ctx.remaining > 0 {
			for len(ctx.extras) > 0 && ctx.remaining > 0 {
				extra := ctx.extras[len(ctx.extras)-1]
				ctx.extras = ctx.extras[0:len(ctx.extras)-1]
				ctx.callback(extra.slice, extra.outputs, nil)
				ctx.remaining--
			}

			task := Task{
				Query: ctx.query,
				Vector: ctx.vector,
				Callback: ctx.ctxCallback,
			}
			for i := 0; i < ctx.perIter; i++ {
				slice := ctx.sampler()
				if slice != nil {
					task.Slices = append(task.Slices, *slice)
				}
			}
			if len(task.Slices) == 0 {
				setErr(fmt.Errorf("sample error"))
				break
			}

			ctx.mu.Unlock()
			tasks.Schedule(task)
			ctx.mu.Lock()
		}
		ctx.running = false
		ctx.mu.Unlock()
	}()
}

func (ctx *TaskContext) Close() {
	ctx.mu.Lock()
	ctx.closed = true
	ctx.mu.Unlock()
}

func (ctx *TaskContext) ctxCallback(slice Slice, outputs [][]DataReader, err error) {
	if err != nil && strings.Contains(err.Error(), "selector reject") {
		log.Printf("[task context] selector reject on slice %v, we will retry", slice)
		return
	}
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.remaining <= 0 {
		ctx.extras = append(ctx.extras, extraOutput{slice, outputs})
	}
	ctx.remaining--
	ctx.callback(slice, outputs, err)
}
