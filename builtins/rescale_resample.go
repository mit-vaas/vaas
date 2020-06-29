package builtins

import (
	"../skyhook"

	"github.com/googollee/go-socket.io"

	"fmt"
	"log"
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

	skyhook.SetupFuncs = append(skyhook.SetupFuncs, func(server *socketio.Server) {
		type TuneRequest struct {
			NodeID int
			Vector string
			MetricSeries int
			MetricNode int
		}

		server.OnConnect("/nodes/rescale-resample", func(s socketio.Conn) error {
			return nil
		})

		server.OnError("/nodes/rescale-resample", func (s socketio.Conn, err error) {
			log.Printf("[socket.io] error on client %v: %v", s.ID(), err)
		})

		server.OnEvent("/nodes/rescale-resample", "tune", func(s socketio.Conn, request TuneRequest) []RescaleResampleConfig {
			origNode := skyhook.GetNode(request.NodeID)
			if origNode == nil || origNode.Type != "rescale-resample" {
				s.Emit("error", "no such rescale-resample node")
				return nil
			}
			vector := skyhook.ParseVector(request.Vector)
			metricSeries := skyhook.GetSeries(request.MetricSeries)
			if metricSeries == nil {
				s.Emit("error", "no such metric series")
				return nil
			}
			metricNode := skyhook.GetNode(request.MetricNode)
			if metricNode == nil {
				s.Emit("error", "no such metric node")
				return nil
			}

			query := skyhook.GetQuery(origNode.QueryID)
			query.Load()

			// load ground truth data
			metricItems := metricSeries.ListItems()
			var gtlist []skyhook.Data
			sliceToIdx := make(map[string]int)
			for i, item := range metricItems {
				data, err := item.Load(item.Slice).Reader().Read(item.Slice.Length())
				if err != nil {
					panic(err)
				}
				gtlist = append(gtlist, data)
				sliceToIdx[item.Slice.String()] = i
			}

			// enumerate the configurations that we want to test
			// only adjust freq if at least one item includes multiple frames
			freqs := []int{1}
			for _, data := range gtlist {
				if data.Length() > 1 {
					freqs = []int{1, 2, 4, 8, 16}
					break
				}
			}
			var cfgs []RescaleResampleConfig
			for _, dims := range [][2]int{{1280, 720}, {960, 540}, {640, 360}, {480, 270}} {
				for _, freq := range freqs {
					cfgs = append(cfgs, RescaleResampleConfig{freq, dims[0], dims[1]})
				}
			}

			go func() {
				for cfgIdx, cfg := range cfgs {
					// TODO: we can parallelize this if we have extra resources
					// to do so, we can do each one in goroutine and set different query IDs
					// so the allocater itself will handle the de-allocation
					skyhook.GetAllocator().Deallocate(skyhook.EnvSetID{"query", query.ID})

					qcopy := *query
					qcopy.Outputs = [][]skyhook.Parent{{skyhook.Parent{
						Type: skyhook.NodeParent,
						NodeID: metricNode.ID,
					}}}
					qcopy.Selector = nil
					nodesCopy := make(map[int]*skyhook.Node)
					for id, node := range qcopy.Nodes {
						nodesCopy[id] = node
					}
					qcopy.Nodes = nodesCopy
					nodeCopy := *origNode
					nodeCopy.Code = string(skyhook.JsonMarshal(cfg))
					qcopy.Nodes[origNode.ID] = &nodeCopy

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

					var mu sync.Mutex
					outputs := make([]skyhook.Data, len(gtlist))
					var wg sync.WaitGroup
					wg.Add(1)
					stream := skyhook.NewExecStream(&qcopy, vector, sampler, 4, func(slice skyhook.Slice, curOutputs [][]skyhook.DataReader, err error) {
						if err != nil {
							if strings.Contains(err.Error(), "sample error") {
								wg.Done()
							} else if err != nil {
								log.Printf("[test] warning: got callback error: %v", err)
							}
							return
						}
						wg.Add(1)
						go func() {
							rd := curOutputs[0][0]
							data, err := rd.Read(slice.Length())
							if err != nil {
								log.Printf("[test] warning: error reading buffer: %v", err)
								return
							}
							mu.Lock()
							idx := sliceToIdx[slice.String()]
							outputs[idx] = skyhook.AdjustDataFreq(data, slice.Length(), rd.Freq(), 1)
							mu.Unlock()
							wg.Done()
						}()
					})
					stream.Get(len(metricItems)+1)
					wg.Wait()

					// compute score
					score := skyhook.Metrics["detectionf1-50"](gtlist, outputs)

					// add up the stats samples after averaging per node
					samples := skyhook.GetAverageStatsByNode(skyhook.GetAllocator().GetContainers(skyhook.EnvSetID{"query", query.ID}))
					var stats skyhook.StatsSample
					for _, sample := range samples {
						stats = stats.Add(sample)
					}

					log.Printf("[rescale-resample] tuning result for %v: score=%v stats=%v", cfg, score, stats)

					type Result struct {
						CfgIdx int
						Score float64
						Stats skyhook.StatsSample
					}

					s.Emit("tune-result", Result{cfgIdx, score, stats})
				}
			}()

			return cfgs
		})
	})
}
