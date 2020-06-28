package builtins

import (
	"../skyhook"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
)

type RescaleResampleConfig struct {
	// target frequency of output buffer
	// (if input buffer freq = 2, and our freq = 4, then we downsample 2x from the input)
	Freq int

	Width int
	Height int
}

type RescaleResample struct {
	node *skyhook.Node
	cfg RescaleResampleConfig
}

func NewRescaleResample(node *skyhook.Node) skyhook.Executor {
	var cfg RescaleResampleConfig
	skyhook.JsonUnmarshal([]byte(node.Code), &cfg)
	if cfg.Freq == 0 {
		cfg.Freq = 1
	}
	return RescaleResample{
		node: node,
		cfg: cfg,
	}
}

func (m RescaleResample) Run(ctx skyhook.ExecContext) skyhook.DataBuffer {
	parents, err := GetParents(ctx, m.node)
	if err != nil {
		return skyhook.GetErrorBuffer(m.node.DataType, fmt.Errorf("rescale-resample error reading parents: %v", err))
	}
	if len(parents) != 1 {
		panic(fmt.Errorf("rescale-resample takes one parent"))
	}
	parent := parents[0]
	var targetFreq int
	if m.cfg.Freq == 0 {
		targetFreq = parent.Freq()
	} else {
		targetFreq = m.cfg.Freq
	}
	expectedLength := (ctx.Slice.Length() + targetFreq-1) / targetFreq

	// push-down the operation if possible
	if vbufReader, ok := parent.(*skyhook.VideoBufferReader); ok {
		vbufReader.Resample(targetFreq / vbufReader.Freq())
		if m.cfg.Width != 0 && m.cfg.Height != 0 {
			vbufReader.Rescale([2]int{m.cfg.Width, m.cfg.Height})
		}
		vbufReader.SetLength(expectedLength)
		return vbufReader
	}

	buf := skyhook.NewSimpleBuffer(parent.Type())
	buf.SetMeta(targetFreq)

	go func() {
		var count int = 0
		err := skyhook.ReadMultiple(ctx.Slice.Length(), targetFreq, parents, func(index int, datas []skyhook.Data) error {
			data := datas[0]
			count += data.Length()
			buf.Write(data)
			return nil
		})
		if err != nil {
			buf.Error(err)
			return
		}
		if count != expectedLength {
			panic(fmt.Errorf("expected resample length %d but got %d", expectedLength, count))
		}
		buf.Close()
	}()

	return buf
}

func (m RescaleResample) Close() {}

func init() {
	skyhook.Executors["rescale-resample"] = NewRescaleResample

	http.HandleFunc("/nodes/rescale-resample", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		r.ParseForm()
		nodeID := skyhook.ParseInt(r.Form.Get("node_id"))
		vectorStr := r.Form.Get("vector")
		metricSeriesID := skyhook.ParseInt(r.Form.Get("metric_series"))
		metricNodeID := skyhook.ParseInt(r.Form.Get("metric_node"))

		origNode := skyhook.GetNode(nodeID)
		if origNode == nil || origNode.Type != "rescale-resample" {
			http.Error(w, "no such rescale-resample node", 404)
			return
		}
		vector := skyhook.ParseVector(vectorStr)
		metricSeries := skyhook.GetSeries(metricSeriesID)
		if metricSeries == nil {
			http.Error(w, "no such series", 404)
			return
		}

		query := skyhook.GetQuery(origNode.QueryID)
		query.Load()

		var cfgs []RescaleResampleConfig
		for _, dims := range [][2]int{{1920, 1080}, {1280, 720}, {960, 540}, {640, 360}, {480, 270}} {
			for _, freq := range []int{1, 2, 4, 8, 16} {
				cfgs = append(cfgs, RescaleResampleConfig{freq, dims[0], dims[1]})
			}
		}

		metricItems := metricSeries.ListItems()

		type Result struct {
			outputs map[string]skyhook.Data
			score float64
			stats skyhook.StatsSample
		}
		results := make(map[RescaleResampleConfig]Result)

		for _, cfg := range cfgs {
			// TODO: we can parallelize this if we have extra resources
			// to do so, we can do each one in goroutine and set different query IDs
			// so the allocater itself will handle the de-allocation
			skyhook.GetAllocator().Deallocate(skyhook.EnvSetID{"query", query.ID})

			qcopy := *query
			qcopy.Outputs = [][]skyhook.Parent{{skyhook.Parent{
				Type: skyhook.NodeParent,
				NodeID: metricNodeID,
			}}}
			qcopy.Selector = nil
			nodesCopy := make(map[int]*skyhook.Node)
			for id, node := range qcopy.Nodes {
				nodesCopy[id] = node
			}
			qcopy.Nodes = nodesCopy
			nodeCopy := *origNode
			nodeCopy.Code = string(skyhook.JsonMarshal(cfg))
			qcopy.Nodes[nodeID] = &nodeCopy

			sampler := func() func() *skyhook.Slice {
				idx := 0
				return func() *skyhook.Slice {
					if idx >= len(metricItems) {
						return nil
					}
					item := metricItems[idx]
					idx++
					return &item.Slice
				}
			}()

			donech := make(chan bool)
			var mu sync.Mutex
			outputs := make(map[string]skyhook.Data)
			stream := skyhook.NewExecStream(&qcopy, vector, sampler, 4, func(slice skyhook.Slice, curOutputs [][]skyhook.DataReader, err error) {
				if err != nil {
					if strings.Contains(err.Error(), "sample error") {
						donech <- true
					} else if err != nil {
						log.Printf("[test] warning: got callback error: %v", err)
					}
					return
				}
				go func() {
					rd := curOutputs[0][0]
					data, err := rd.Read(slice.Length())
					if err != nil {
						log.Printf("[test] warning: error reading buffer: %v", err)
						return
					}
					mu.Lock()
					outputs[slice.String()] = skyhook.AdjustDataFreq(data, slice.Length(), rd.Freq(), 1)
					mu.Unlock()
				}()
			})
			stream.Get(len(metricItems)+1)
			<- donech

			// compute score
			var score float64 = 1

			// average the stats samples
			// TODO: need an allocator.GetAllContainers function
			var samples []skyhook.StatsSample
			for _, container := range skyhook.GetAllocator().GetContainers(query.GetEnvSet()) {
				var sample skyhook.StatsSample
				err := skyhook.JsonPost(container.BaseURL, "/allstats", nil, &sample)
				if err != nil {
					panic(err)
				}
				samples = append(samples, sample)
			}

			results[cfg] = Result{outputs, score, skyhook.AverageStats(samples)}
			log.Println("got for cfg", cfg, score, skyhook.AverageStats(samples))
		}
	})
}
