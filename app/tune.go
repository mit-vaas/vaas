package app

import (
	"../vaas"

	"github.com/googollee/go-socket.io"

	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
)

func init() {
	http.HandleFunc("/tune/tunable-nodes", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		queryID := vaas.ParseInt(r.Form.Get("query_id"))
		query := GetQuery(queryID)
		if query == nil {
			http.Error(w, "no such query", 404)
			return
		}
		query.Load()
		tunableNodes := []vaas.Node{}
		for _, node := range query.Nodes {
			execMeta := vaas.Executors[node.Type]
			if execMeta.Tune == nil {
				continue
			}
			tunableNodes = append(tunableNodes, *node)
		}
		vaas.JsonResponse(w, tunableNodes)
	})

	SetupFuncs = append(SetupFuncs, func(server *socketio.Server) {
		type TuneRequest struct {
			NodeIDs []int
			Vector string
			MetricSeries int
			MetricNode int

			// specifies a metric in metric.go, like "detection-f1"
			// or, "direct" indicating that MetricNode outputs the float score directly
			Metric string
		}

		// represents (possibly pending) results for one combination of configurations
		type TuneUpdate struct {
			Idx int

			// list of nodes and corresponding descriptions of their configurations for this TuneUpdate
			Nodes []*DBNode
			cfgs []string
			Description []string

			// output of the metric
			Score float64
			Stats vaas.StatsSample
		}

		server.OnConnect("/tune", func(s socketio.Conn) error {
			return nil
		})

		server.OnError("/tune", func (s socketio.Conn, err error) {
			log.Printf("[socket.io: tune] error on client %v: %v", s.ID(), err)
		})

		server.OnEvent("/tune", "tune", func(s socketio.Conn, request TuneRequest) []TuneUpdate {
			reportErr := func(err string) {
				log.Printf("[tune] error: %s", err)
				s.Emit("error", err)
			}

			log.Printf("[tune] got tune request %v", request)
			var nodes []*DBNode
			for _, id := range request.NodeIDs {
				node := GetNode(id)
				if node == nil {
					reportErr(fmt.Sprintf("no such node %d", id))
					return nil
				} else if vaas.Executors[node.Type].Tune == nil {
					reportErr(fmt.Sprintf("type %s (at node %s) is not tunable", node.Type, node.Name))
					return nil
				}
				nodes = append(nodes, node)
			}
			vector := ParseVector(request.Vector)
			metricSeries := GetSeries(request.MetricSeries)
			if metricSeries == nil {
				reportErr("no such metric series")
				return nil
			}
			metricNode := GetNode(request.MetricNode)
			if metricNode == nil {
				reportErr("no such metric node")
				return nil
			}

			// direct means the metricNode outputs the float score
			// otherwise we use a metric to compare the metricNode output to ground truth data
			// if direct, the input series needs to include the ground truth data
			if request.Metric == "direct" {
				vector = append(vector, metricSeries)
			}

			query := GetQuery(nodes[0].QueryID)
			query.Load()

			// load ground truth data
			var metricItems []DBItem
			var gtlist []vaas.Data
			sliceToIdx := make(map[string]int)
			for _, item := range metricSeries.ListItems() {
				// if same slice is labeled twice, we need to skip it
				if _, ok := sliceToIdx[item.Slice.String()]; ok {
					continue
				}

				data, err := item.Load(item.Slice).Reader().Read(item.Slice.Length())
				if err != nil {
					panic(err)
				}
				sliceToIdx[item.Slice.String()] = len(metricItems)
				metricItems = append(metricItems, item)
				gtlist = append(gtlist, data)
			}

			// enumerate the configurations that we want to test
			var tuneUpdates []TuneUpdate
			var recursiveEnumerate func(int, [2][]string)
			recursiveEnumerate = func(nodeIdx int, partial [2][]string) {
				if nodeIdx >= len(nodes) {
					tuneUpdates = append(tuneUpdates, TuneUpdate{
						Idx: len(tuneUpdates),
						Nodes: nodes,
						cfgs: partial[0],
						Description: partial[1],
					})
					return
				}

				node := nodes[nodeIdx]
				for _, cfg := range vaas.Executors[node.Type].Tune(node.Node, gtlist) {
					partialCopy := [2][]string{
						make([]string, len(partial[0])+1),
						make([]string, len(partial[1])+1),
					}
					copy(partialCopy[0], partial[0])
					copy(partialCopy[1], partial[1])
					partialCopy[0][nodeIdx] = cfg[0]
					partialCopy[1][nodeIdx] = cfg[1]
					recursiveEnumerate(nodeIdx+1, partialCopy)
				}
			}
			recursiveEnumerate(0, [2][]string{nil, nil})

			log.Printf("[tune] testing %d configurations", len(tuneUpdates))

			go func() {
				virtualQueryID := -int(rand.Int63())
				for _, update := range tuneUpdates {
					// TODO: we can parallelize this if we have extra resources
					// to do so, we can do each one in goroutine and set different query IDs
					// so the allocater itself will handle the de-allocation
					GetAllocator().Deallocate(vaas.EnvSetID{"query", virtualQueryID})

					qcopy := *query
					qcopy.ID = virtualQueryID
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
					for i, node := range nodes {
						nodeCopy := node.Node
						nodeCopy.Code = update.cfgs[i]
						qcopy.Nodes[nodeCopy.ID] = &nodeCopy
					}

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
					opts := vaas.ExecOptions{
						NoPersist: true,
						//IgnoreItems: true,
					}
					stream := NewExecStream(&qcopy, vector, sampler, 4, opts, func(slice vaas.Slice, curOutputs [][]vaas.DataReader, err error) {
						if err != nil {
							log.Printf("[test] warning: got callback error: %v", err)
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
					stream.Get(len(metricItems))
					stream.Wait()
					wg.Wait()

					// compute score
					var score float64
					if request.Metric == "direct" {
						// average the FloatData outputs
						var sum float64 = 0
						var count int = 0
						for _, output := range outputs {
							fdata := output.(vaas.FloatData)
							for _, v := range fdata {
								sum += v
								count++
							}
						}
						score = sum / float64(count)
					} else {
						score = Metrics[request.Metric](gtlist, outputs)
					}

					// add up the stats samples after averaging per node
					samples := vaas.GetStatsByNode(GetAllocator().GetContainers(vaas.EnvSetID{"query", virtualQueryID}))
					var stats vaas.StatsSample
					for _, sample := range samples {
						stats = stats.Add(sample)
					}

					log.Printf("[tune] tuning result for %v: score=%v stats=%v", update.Description, score, stats)
					update.Score = score
					update.Stats = stats
					s.Emit("tune-result", update)
				}
			}()

			return tuneUpdates
		})
	})
}
