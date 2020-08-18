package app

import (
	"../vaas"
	gouuid "github.com/google/uuid"

	"fmt"
	"log"
	"strings"
	"sync"
)

func (query *DBQuery) GetEnvSet() vaas.EnvSet {
	envSetID := vaas.EnvSetID{"query", query.ID}
	var environments []vaas.Environment
	environments = append(environments, vaas.Environment{
		Template: "default",
		Requirements: map[string]int{
			"container": 1,
		},
	})
	for _, node := range query.Nodes {
		meta := vaas.Executors[node.Type]
		if meta.Environment != nil {
			env := *meta.Environment
			env.RefID = node.ID
			environments = append(environments, env)
		}
	}
	return vaas.EnvSet{envSetID, environments}
}

func (query *DBQuery) Allocate(vector []*DBSeries, slice vaas.Slice) vaas.ExecContext {
	query.Load()
	uuid := gouuid.New().String()

	containers := allocator.Pick(query.GetEnvSet().ID)
	if containers == nil {
		// make sure query is fresh since we need to allocate
		query.Reload()
		containers = allocator.Allocate(query.GetEnvSet())
	}

	// figure out which nodes should be on which containers
	// container.env.refid specifies the node ID for non-default containers
	// so we assign nodes with their own container, then assign rest to default
	nodeContainers := make(map[int]vaas.Container)
	var defaultContainer vaas.Container
	for _, container := range containers {
		if container.Environment.Template == "default" {
			defaultContainer = container
		} else {
			nodeContainers[container.Environment.RefID] = container
		}
	}
	for _, node := range query.Nodes {
		if _, ok := nodeContainers[node.ID]; !ok {
			nodeContainers[node.ID] = defaultContainer
		}
	}

	context := vaas.ExecContext{
		Nodes: query.Nodes,
		Containers: nodeContainers,
		UUID: uuid,
		Items: make(map[int]*vaas.Item),
		Slice: slice,
	}
	for _, series := range vector {
		context.Vector = append(context.Vector, series.Series)
	}

	// find items that already exist on disk
	for _, node := range query.Nodes {
		vn := GetOrCreateVNode(&DBNode{Node: *node}, vector)
		if vn.Series == nil {
			continue
		}
		item := DBSeries{Series: *vn.Series}.GetItem(slice)
		if item == nil {
			continue
		}
		context.Items[node.ID] = &item.Item
	}

	// collect the input items
	for _, series := range vector {
		item := series.GetItem(slice)
		if item == nil {
			panic(fmt.Errorf("no item for vector input %s", series.Name))
		}
		context.Inputs = append(context.Inputs, item.Item)
	}

	return context
}

func (query *DBQuery) RunBuffer(vector []*DBSeries, slice vaas.Slice, opts vaas.ExecOptions) ([][]vaas.DataBuffer, error) {
	context := query.Allocate(vector, slice)
	context.Opts = opts
	defer context.Release()

	if context.Opts.IgnoreItems {
		context.Items = nil
	}

	var selector *vaas.Node
	var outputs [][]vaas.Parent
	if opts.Selector != nil {
		selector = opts.Selector
	} else if !opts.NoSelector {
		selector = query.Selector
	}
	if opts.Outputs != nil {
		outputs = opts.Outputs
	} else {
		outputs = query.Outputs
	}

	// get the selector
	if selector != nil {
		rd, err := context.GetReader(*selector)
		if err != nil {
			log.Printf("[query-run %s %v] error computing selector: %v", query.Name, slice, err)
			return nil, err
		}
		data, err := rd.Read(slice.Length())
		if err != nil {
			log.Printf("[query-run %s %v] error computing selector: %v", query.Name, slice, err)
			return nil, err
		}
		if data.IsEmpty() {
			return nil, fmt.Errorf("selector reject")
		}
	}

	// get the outputs for rendering
	// TODO: think about whether rendering should be its own node
	// currently caller does the rendering
	buffers := make([][]vaas.DataBuffer, len(outputs))
	for i := range outputs {
		for _, output := range outputs[i] {
			if output.Type == vaas.NodeParent {
				buf, err := context.GetBuffer(*query.Nodes[output.NodeID])
				if err != nil {
					log.Printf("[query-run %s %v] error computing outputs: %v", query.Name, slice, err)
					return nil, err
				}
				buffers[i] = append(buffers[i], buf)
			} else if output.Type == vaas.SeriesParent {
				buf := &vaas.VideoFileBuffer{context.Inputs[output.SeriesIdx], slice}
				buffers[i] = append(buffers[i], buf)
			}
		}
	}
	return buffers, nil
}

func (query *DBQuery) Run(vector []*DBSeries, slice vaas.Slice, opts vaas.ExecOptions) ([][]vaas.DataReader, error) {
	buffers, err := query.RunBuffer(vector, slice, opts)
	if err != nil {
		return nil, err
	}
	readers := make([][]vaas.DataReader, len(buffers))
	for i := range buffers {
		readers[i] = make([]vaas.DataReader, len(buffers[i]))
		for j, buf := range buffers[i] {
			readers[i][j] = buf.Reader()
		}
	}
	return readers, nil
}

type extraOutput struct {
	slice vaas.Slice
	outputs [][]vaas.DataReader
}

type ExecStream struct {
	query *DBQuery
	vector []*DBSeries
	sampler func() *vaas.Slice
	perIter int
	opts vaas.ExecOptions
	callback func(vaas.Slice, [][]vaas.DataReader, error)
	seenSegments map[string]bool
	mu sync.Mutex
	cond *sync.Cond

	// number of background query exec goroutines that are running
	running int
	closed bool

	remaining int
	finished int

	// extra outputs we haven't sent to callback
	extras []extraOutput
}

func NewExecStream(query *DBQuery, vector []*DBSeries, sampler func() *vaas.Slice, perIter int, opts vaas.ExecOptions, callback func(vaas.Slice, [][]vaas.DataReader, error)) *ExecStream {
	query.Load()
	stream := &ExecStream{
		query: query,
		vector: vector,
		sampler: sampler,
		perIter: perIter,
		seenSegments: make(map[string]bool),
		opts: opts,
		callback: callback,
	}
	stream.cond = sync.NewCond(&stream.mu)
	return stream
}

// caller must have lock
func (ctx *ExecStream) setErr(err error) {
	for i := 0; i < ctx.remaining; i++ {
		ctx.callback(vaas.Slice{}, nil, err)
	}
	ctx.remaining = 0
}

func (ctx *ExecStream) tryOne(query *DBQuery, slice vaas.Slice) {
	outputs, err := query.Run(ctx.vector, slice, ctx.opts)
	if err != nil && strings.Contains(err.Error(), "selector reject") {
		log.Printf("[task context] selector reject on slice %v, we will retry", slice)
		ctx.callback(slice, outputs, err)
		return
	}
	ctx.mu.Lock()
	if ctx.remaining <= 0 {
		ctx.extras = append(ctx.extras, extraOutput{slice, outputs})
		ctx.mu.Unlock()
		return
	}
	ctx.remaining--
	ctx.callback(slice, outputs, err)
	ctx.mu.Unlock()

	for _, l := range outputs {
		for _, rd := range l {
			rd.Wait()
		}
	}
}

func (ctx *ExecStream) backgroundThread() {
	// create local copy of DBQuery since it's not thread safe
	query := new(DBQuery)
	*query = *ctx.query

	ctx.mu.Lock()
	for !ctx.closed && ctx.remaining > 0 {
		if len(ctx.extras) > 0 {
			for len(ctx.extras) > 0 && ctx.remaining > 0 {
				extra := ctx.extras[len(ctx.extras)-1]
				ctx.extras = ctx.extras[0:len(ctx.extras)-1]
				ctx.callback(extra.slice, extra.outputs, nil)
				ctx.remaining--
			}
			continue
		}

		// reuse perIter to decide how many slices to try before giving up
		var slice *vaas.Slice
		for i := 0; i < ctx.perIter; i++ {
			slice = ctx.sampler()
			if slice == nil {
				continue
			}
			break
		}
		if slice == nil {
			// set error if we are the last thread
			// setting error will notify the callback that all the ctx.remaining have failed
			// so if we aren't the last thread, we must not set error since callback expects response for running threads
			if ctx.running == 1 {
				ctx.setErr(fmt.Errorf("sample error"))
			}
			break
		}

		ctx.mu.Unlock()
		ctx.tryOne(query, *slice)
		ctx.mu.Lock()
	}

	ctx.running--
	ctx.cond.Broadcast()
	ctx.mu.Unlock()
}

func (ctx *ExecStream) Get(n int) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.closed {
		ctx.setErr(fmt.Errorf("task context is closed"))
		return
	}
	ctx.remaining += n

	// spawn perIter goroutines to get outputs simultaneously
	for ctx.running < ctx.perIter {
		ctx.running++
		go ctx.backgroundThread()
	}
}

func (ctx *ExecStream) Close() {
	ctx.mu.Lock()
	ctx.closed = true
	ctx.mu.Unlock()
}

func (ctx *ExecStream) Wait() {
	ctx.mu.Lock()
	for ctx.running > 0 {
		ctx.cond.Wait()
	}
	ctx.mu.Unlock()
}
