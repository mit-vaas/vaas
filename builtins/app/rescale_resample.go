package builtins_app

import (
	"../../builtins"
	"../../app"
	"../../vaas"

	"github.com/googollee/go-socket.io"

	"log"
	"strings"
	"sync"
)

func init() {
	app.SetupFuncs = append(app.SetupFuncs, func(server *socketio.Server) {
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

		server.OnEvent("/nodes/rescale-resample", "tune", func(s socketio.Conn, request TuneRequest) []builtins.RescaleResampleConfig {
			origNode := app.GetNode(request.NodeID)
			if origNode == nil || origNode.Type != "rescale-resample" {
				s.Emit("error", "no such rescale-resample node")
				return nil
			}
			vector := app.ParseVector(request.Vector)
			metricSeries := app.GetSeries(request.MetricSeries)
			if metricSeries == nil {
				s.Emit("error", "no such metric series")
				return nil
			}
			metricNode := app.GetNode(request.MetricNode)
			if metricNode == nil {
				s.Emit("error", "no such metric node")
				return nil
			}

			query := app.GetQuery(origNode.QueryID)
			query.Load()

			// load ground truth data
			metricItems := metricSeries.ListItems()
			var gtlist []vaas.Data
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
			var cfgs []builtins.RescaleResampleConfig
			for _, dims := range [][2]int{{1280, 720}, {960, 540}, {640, 360}, {480, 270}} {
				for _, freq := range freqs {
					cfgs = append(cfgs, builtins.RescaleResampleConfig{freq, dims[0], dims[1]})
				}
			}

			go func() {
				for cfgIdx, cfg := range cfgs {
					// TODO: we can parallelize this if we have extra resources
					// to do so, we can do each one in goroutine and set different query IDs
					// so the allocater itself will handle the de-allocation
					app.GetAllocator().Deallocate(vaas.EnvSetID{"query", query.ID})

					qcopy := *query
					qcopy.Outputs = [][]vaas.Parent{{vaas.Parent{
						Type: vaas.NodeParent,
						NodeID: metricNode.ID,
					}}}
					qcopy.Selector = nil
					nodesCopy := make(map[int]*vaas.Node)
					for id, node := range qcopy.Nodes {
						nodesCopy[id] = node
					}
					qcopy.Nodes = nodesCopy
					nodeCopy := *origNode
					nodeCopy.Code = string(vaas.JsonMarshal(cfg))
					qcopy.Nodes[origNode.ID] = &nodeCopy.Node

					sampler := func() func() *vaas.Slice {
						idx := 0
						return func() *vaas.Slice {
							if idx >= len(metricItems) {
								return nil
							}
							item := metricItems[idx]
							idx++
							return &item.Slice
						}
					}()

					var mu sync.Mutex
					outputs := make([]vaas.Data, len(gtlist))
					var wg sync.WaitGroup
					wg.Add(1)
					stream := app.NewExecStream(&qcopy, vector, sampler, 4, func(slice vaas.Slice, curOutputs [][]vaas.DataReader, err error) {
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
							outputs[idx] = vaas.AdjustDataFreq(data, slice.Length(), rd.Freq(), 1)
							mu.Unlock()
							wg.Done()
						}()
					})
					stream.Get(len(metricItems)+1)
					wg.Wait()

					// compute score
					score := app.Metrics["detectionf1-50"](gtlist, outputs)

					// add up the stats samples after averaging per node
					samples := vaas.GetAverageStatsByNode(app.GetAllocator().GetContainers(vaas.EnvSetID{"query", query.ID}))
					var stats vaas.StatsSample
					for _, sample := range samples {
						stats = stats.Add(sample)
					}

					log.Printf("[rescale-resample] tuning result for %v: score=%v stats=%v", cfg, score, stats)

					type Result struct {
						CfgIdx int
						Score float64
						Stats vaas.StatsSample
					}

					s.Emit("tune-result", Result{cfgIdx, score, stats})
				}
			}()

			return cfgs
		})
	})
}
