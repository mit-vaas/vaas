package app

import (
	"../vaas"
	gouuid "github.com/google/uuid"

	"fmt"
	"log"
	"net/http"
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
	return vaas.EnvSet{envSetID, environments}
}

func (query *DBQuery) Run(vector []*DBSeries, slice vaas.Slice) ([][]vaas.DataReader, error) {
	uuid := gouuid.New().String()

	containers := allocator.Allocate(query.GetEnvSet())
	nodeContainers := make(map[int]vaas.Container)
	for _, node := range query.Nodes {
		nodeContainers[node.ID] = containers[0]
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

	// make sure we release the associated buffers after we return
	// (we won't be done rendering the video, but the containers can remove the global
	// reference to the buffers since the HTTP calls for the rendering will already have
	// been initialized)
	defer func() {
		for _, container := range containers {
			resp, _ := http.Post(container.BaseURL + "/query/finish?uuid=" + uuid, "", nil)
			if resp.Body != nil {
				resp.Body.Close()
			}
		}
	}()

	// get the selector
	selector := query.Selector
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
	outputs := make([][]vaas.DataReader, len(query.Outputs))
	for i := range query.Outputs {
		for _, output := range query.Outputs[i] {
			if output.Type == vaas.NodeParent {
				rd, err := context.GetReader(*query.Nodes[output.NodeID])
				if err != nil {
					log.Printf("[query-run %s %v] error computing outputs: %v", query.Name, slice, err)
					return nil, err
				}
				outputs[i] = append(outputs[i], rd)
			} else if output.Type == vaas.SeriesParent {
				buf := &vaas.VideoFileBuffer{context.Inputs[output.SeriesIdx], slice}
				outputs[i] = append(outputs[i], buf.Reader())
			}
		}
	}
	return outputs, nil
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
	callback func(vaas.Slice, [][]vaas.DataReader, error)
	seenSegments map[string]bool
	mu sync.Mutex

	running bool
	closed bool

	remaining int
	finished int

	// extra outputs we haven't sent to callback
	extras []extraOutput
}

func NewExecStream(query *DBQuery, vector []*DBSeries, sampler func() *vaas.Slice, perIter int, callback func(vaas.Slice, [][]vaas.DataReader, error)) *ExecStream {
	return &ExecStream{
		query: query,
		vector: vector,
		sampler: sampler,
		perIter: perIter,
		seenSegments: make(map[string]bool),
		callback: callback,
	}
}

func (ctx *ExecStream) Get(n int) {
	// called while we have lock
	setErr := func(err error) {
		for i := 0; i < ctx.remaining; i++ {
			ctx.callback(vaas.Slice{}, nil, err)
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

			any := false
			var wg sync.WaitGroup
			for i := 0; i < ctx.perIter; i++ {
				slice := ctx.sampler()
				if slice != nil {
					any = true
					wg.Add(1)
					go ctx.tryOne(*slice, &wg)
				}
			}
			if !any {
				setErr(fmt.Errorf("sample error"))
				break
			}

			ctx.mu.Unlock()
			wg.Wait()
			ctx.mu.Lock()
		}
		ctx.running = false
		ctx.mu.Unlock()
	}()
}

func (ctx *ExecStream) Close() {
	ctx.mu.Lock()
	ctx.closed = true
	ctx.mu.Unlock()
}

func (ctx *ExecStream) tryOne(slice vaas.Slice, wg *sync.WaitGroup) {
	defer wg.Done()
	outputs, err := ctx.query.Run(ctx.vector, slice)
	if err != nil && strings.Contains(err.Error(), "selector reject") {
		log.Printf("[task context] selector reject on slice %v, we will retry", slice)
		ctx.callback(slice, outputs, err)
		return
	}
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.remaining <= 0 {
		ctx.extras = append(ctx.extras, extraOutput{slice, outputs})
		return
	}
	ctx.remaining--
	ctx.callback(slice, outputs, err)
}
