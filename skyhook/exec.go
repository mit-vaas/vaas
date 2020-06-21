package skyhook

import (
	"github.com/googollee/go-socket.io"
	"github.com/google/uuid"

	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ParentType string
const (
	NodeParent ParentType = "n"
	SeriesParent = "s"
)
type Parent struct {
	Spec string
	Type ParentType
	NodeID int
	SeriesIdx int
}

func ParseParents(str string) []Parent {
	parents := []Parent{}
	if str == "" {
		return parents
	}
	for _, part := range strings.Split(str, ",") {
		var parent Parent
		parent.Spec = part
		parent.Type = ParentType(part[0:1])
		if parent.Type == NodeParent {
			parent.NodeID = ParseInt(part[1:])
		} else if parent.Type == SeriesParent {
			parent.SeriesIdx = ParseInt(part[1:])
		}
		parents = append(parents, parent)
	}
	return parents
}

type Executor interface {
	Run(parents []DataReader, slice Slice) DataBuffer
	Close()
}

var Executors = map[string]func(*Node, *Query) Executor{}

type Node struct {
	ID int
	Name string
	ParentTypes []DataType
	Parents []Parent
	Type DataType
	Ext string
	Code string
	QueryID int

}

const NodeQuery = "SELECT id, name, parent_types, parents, type, ext, code, query_id FROM nodes"

func nodeListHelper(rows *Rows) []*Node {
	nodes := []*Node{}
	for rows.Next() {
		var node Node
		var parentTypes, parents string
		rows.Scan(&node.ID, &node.Name, &parentTypes, &parents, &node.Type, &node.Ext, &node.Code, &node.QueryID)
		if parentTypes != "" {
			for _, dt := range strings.Split(parentTypes, ",") {
				node.ParentTypes = append(node.ParentTypes, DataType(dt))
			}
		}
		node.Parents = ParseParents(parents)
		nodes = append(nodes, &node)
	}
	return nodes
}

func ListNodesByQuery(query *Query) []*Node {
	rows := db.Query(NodeQuery + " WHERE query_id = ?", query.ID)
	return nodeListHelper(rows)
}

func GetNode(id int) *Node {
	rows := db.Query(NodeQuery + " WHERE id = ?", id)
	nodes := nodeListHelper(rows)
	if len(nodes) == 1 {
		return nodes[0]
	} else {
		return nil
	}
}

func (node *Node) ListVNodes() []*VNode {
	return ListVNodesWithNode(node)
}

func (node *Node) Exec(query *Query) Executor {
	return Executors[node.Ext](node, query)
}

func (node *Node) GetChildren(m map[int]*Node) []*Node {
	var nodes []*Node
	for _, other := range m {
		for _, parent := range other.Parents {
			if parent.Type != NodeParent {
				continue
			}
			if parent.NodeID != node.ID {
				continue
			}
			nodes = append(nodes, other)
		}
	}
	return nodes
}

func (node *Node) OnChange() {
	// delete all saved labels at the vnodes for this node
	// and recursively delete for vnodes that depend on that vnode

	query := GetQuery(node.QueryID)
	query.Load()

	affectedNodes := map[int]*Node{node.ID: node}
	queue := []*Node{node}
	for len(queue) > 0 {
		head := queue[0]
		queue = queue[1:]
		// find all children of head
		for _, node := range head.GetChildren(query.Nodes) {
			if affectedNodes[node.ID] != nil {
				continue
			}
			affectedNodes[node.ID] = node
			queue = append(queue, node)
		}
	}

	for _, node := range affectedNodes {
		for _, vn := range node.ListVNodes() {
			vn.Clear()
		}
	}
}

func (node *Node) Remove() {
	node.OnChange()
	db.Exec("DELETE FROM nodes WHERE id = ?", node.ID)
}

type VNode struct {
	ID int
	NodeID int
	VectorStr string
	SeriesID *int

	Vector []*Series
	Node *Node
	Series *Series
}

const VNodeQuery = "SELECT id, node_id, vector, series_id FROM vnodes"

func vnodeListHelper(rows *Rows) []*VNode {
	var vnodes []*VNode
	for rows.Next() {
		var vn VNode
		rows.Scan(&vn.ID, &vn.NodeID, &vn.VectorStr, &vn.SeriesID)
		vnodes = append(vnodes, &vn)
	}
	return vnodes
}

func GetVNode(node *Node, vector []*Series) *VNode {
	rows := db.Query(VNodeQuery + " WHERE vector = ? AND node_id = ?", Vector(vector).String(), node.ID)
	vnodes := vnodeListHelper(rows)
	if len(vnodes) == 1 {
		vnodes[0].Load()
		return vnodes[0]
	} else {
		return nil
	}
}

func GetOrCreateVNode(node *Node, vector []*Series) *VNode {
	vn := GetVNode(node, vector)
	if vn != nil {
		return vn
	}
	db.Exec("INSERT OR REPLACE INTO vnodes (node_id, vector) VALUES (?, ?)", node.ID, Vector(vector).String())
	return GetVNode(node, vector)
}

func ListVNodesWithNode(node *Node) []*VNode {
	rows := db.Query(VNodeQuery + " WHERE node_id = ?", node.ID)
	return vnodeListHelper(rows)
}

func (vn *VNode) Load() {
	if len(vn.Vector) > 0 || vn.Node != nil || vn.Series != nil {
		return
	}
	vn.Vector = ParseVector(vn.VectorStr)
	vn.Node = GetNode(vn.NodeID)
	if vn.SeriesID != nil {
		vn.Series = GetSeries(*vn.SeriesID)
	}
}

// clear saved labels at a node
func (vn *VNode) Clear() {
	vn.Load()
	if vn.Series != nil {
		log.Printf("[exec (%s)] clearing outputs series %s", vn.Node.Name, vn.Series.Name)
		vn.Series.Clear()
	}
}

type Plan struct {
	// video -> classification nodes that specify whether the segment should be skipped or not
	// (e.g. probabilistic predicates)
	// there could be multiple nodes but they should be combined with OR/AND into one node
	SkipOptimization *Node

	// these replace the outputs of certain nodes with an alternative node
	// for example, MIRIS replaces the output of a track predicate node with its own node
	// this node may internally use predecessor nodes in the query graph, such as detection
	//   and tracking steps, but the predecessors are not called by the system
	ReplaceOptimizations map[int]*Node

	// for now, model/resolution auto-selection seems better to implement as a special node
	// that decides what to do instead of as part of the execution plan.
	// but we could always implement it as a ReplaceOptimization if desired.
}

// Metadata for rendering query and its nodes on the front-end query editor.
type QueryRenderMeta map[string][2]int

type Query struct {
	ID int
	Name string
	Plan *Plan
	OutputsStr string
	SelectorID *int
	RenderMeta QueryRenderMeta

	Outputs [][]Parent
	Selector *Node

	Nodes map[int]*Node
}

const QueryQuery = "SELECT id, name, outputs, selector, render_meta FROM queries"

func queryListHelper(rows *Rows) []*Query {
	queries := []*Query{}
	for rows.Next() {
		var query Query
		var renderMeta string
		rows.Scan(&query.ID, &query.Name, &query.OutputsStr, &query.SelectorID, &renderMeta)
		if renderMeta != "" {
			JsonUnmarshal([]byte(renderMeta), &query.RenderMeta)
		}
		queries = append(queries, &query)
	}
	return queries
}

func GetQuery(queryID int) *Query {
	rows := db.Query(QueryQuery + " WHERE id = ?", queryID)
	queries := queryListHelper(rows)
	if len(queries) == 1 {
		return queries[0]
	} else {
		return nil
	}
}

func ListQueries() []*Query {
	rows := db.Query(QueryQuery)
	return queryListHelper(rows)
}

func (query *Query) FlatOutputs() []*Node {
	query.Load()
	var flat []*Node
	for _, outputs := range query.Outputs {
		for _, output := range outputs {
			if output.Type != NodeParent {
				continue
			}
			flat = append(flat, query.Nodes[output.NodeID])
		}
	}
	return flat
}

func (query *Query) Load() {
	if query.Nodes != nil {
		return
	}

	query.Nodes = make(map[int]*Node)
	for _, node := range ListNodesByQuery(query) {
		query.Nodes[node.ID] = node
	}

	// load output and selector
	if query.OutputsStr != "" {
		sections := strings.Split(query.OutputsStr, ";")
		query.Outputs = make([][]Parent, len(sections))
		for i, section := range sections {
			query.Outputs[i] = ParseParents(section)
		}
	}
	if query.SelectorID != nil {
		query.Selector = query.Nodes[*query.SelectorID]
	}
}

func (query *Query) GetOutputVectors(inputs []*Series) [][]*Series {
	query.Load()
	outputs := make([][]*Series, len(query.Outputs))
	for i, l := range query.Outputs {
		for _, output := range l {
			if output.Type == NodeParent {
				node := query.Nodes[output.NodeID]
				vn := GetOrCreateVNode(node, inputs)
				outputs[i] = append(outputs[i], vn.Series)
			} else if output.Type == SeriesParent {
				outputs[i] = append(outputs[i], inputs[output.SeriesIdx])
			}
		}
	}
	return outputs
}

type NodeStats struct {
	samples []time.Duration
}

// mean time in seconds
func (s NodeStats) Mean() float64 {
	if len(s.samples) == 0 {
		return 0
	}
	var sum float64 = 0
	for _, x := range s.samples {
		sum += x.Seconds()
	}
	return sum / float64(len(s.samples))
}

type QueryExecutor struct {
	query *Query

	// map from node ID to executors
	executors map[int]Executor

	wg sync.WaitGroup
	stats map[int]NodeStats
}

func NewQueryExecutor(query *Query) *QueryExecutor {
	query.Load()
	return &QueryExecutor{
		query: query,
		executors: make(map[int]Executor),
		stats: make(map[int]NodeStats),
	}
}

func (e *QueryExecutor) Run(vector []*Series, slice Slice, callback func([][]DataReader, error)) {
	/*
	// check what the skip optimization says about it
	skipBuffers := GetVNode(query.Plan.SkipOptimization, slices[0].Clip.Video).Test(query, slices)
	for i := range skipBuffers {
		go func(i int) {
			data, err := skipBuffers[i].ReadFull()
			if err != nil {
				outputs[i].Error(err)
				return
			}
			any := false
			for _, cls := range data.Classes {
				if cls != 0 {
					any = true
					break
				}
			}
			if !any {
				outputs[i].Error(fmt.Errorf("skipped"))
				return
			}
			buf := vn.Test(query, []ClipSlice{slices[i]})[0]
			outputs[i].CopyFrom(buf)
		}(i)
	}

	// if replaced by optimization, then run the optimization node instead
	if query.ReplaceOptimizations[node.ID] != nil {
		vn := GetVNode(query.ReplaceOptimizations[node.ID], slices[0].Clip.Video)
		return vn.Exec(query, slices)
	}
	*/

	// create map (node ID -> outputs at that node) for caching outputs
	// we will populate this either by loading label or by running node
	cachedInputs := make([]DataBuffer, len(vector))
	cachedOutputs := make(map[int]DataBuffer)

	getInputBuffer := func(idx int) DataBuffer {
		if cachedInputs[idx] == nil {
			item := vector[idx].GetItem(slice)
			if item == nil {
				panic(fmt.Errorf("no item for vector input %s", vector[idx].Name))
			}
			cachedInputs[idx] = &VideoFileBuffer{*item, slice}
		}
		return cachedInputs[idx]
	}

	collectOutputs := func() [][]DataReader {
		outputs := make([][]DataReader, len(e.query.Outputs))
		for i := range e.query.Outputs {
			for _, output := range e.query.Outputs[i] {
				if output.Type == NodeParent {
					outputs[i] = append(outputs[i], cachedOutputs[output.NodeID].Reader())
				} else if output.Type == SeriesParent {
					outputs[i] = append(outputs[i], getInputBuffer(output.SeriesIdx).Reader())
				}
			}
		}
		return outputs
	}

	// (1) load labels that we already have
	loadLabels := func(node *Node) bool {
		if cachedOutputs[node.ID] != nil {
			return true
		}
		vn := GetOrCreateVNode(node, vector)
		if vn.Series == nil {
			return false
		}
		item := vn.Series.GetItem(slice)
		if item == nil {
			return false
		}
		cachedOutputs[node.ID] = item.Load(slice)
		return true
	}

	run := func(targets []*Node) {
		q := targets
		seen := make(map[int]bool)
		needed := make(map[int]bool)
		for _, node := range q {
			seen[node.ID] = true
		}
		for len(q) > 0 {
			node := q[0]
			q = q[1:]
			ok := loadLabels(node)
			if ok {
				continue
			}
			needed[node.ID] = true
			for _, parent := range node.Parents {
				if parent.Type != NodeParent {
					continue
				} else if seen[parent.NodeID] {
					continue
				}
				seen[parent.NodeID] = true
				q = append(q, e.query.Nodes[parent.NodeID])
			}
		}
		if len(needed) == 0 {
			return
		}

		collectParents := func(node *Node) []DataReader {
			parents := []DataReader{}
			for _, parent := range node.Parents {
				if parent.Type == SeriesParent {
					parents = append(parents, getInputBuffer(parent.SeriesIdx).Reader())
				} else if parent.Type == NodeParent {
					buf := cachedOutputs[parent.NodeID]
					if buf == nil {
						return nil
					}
					parents = append(parents, buf.Reader())
				}
			}
			return parents
		}

		// (2) repeatedly run nodes where parents are all there, until no more needed
		for len(needed) > 0 {
			for nodeID := range needed {
				node := e.query.Nodes[nodeID]
				parents := collectParents(node)
				if parents == nil {
					continue
				}
				if e.executors[nodeID] == nil {
					e.executors[nodeID] = node.Exec(e.query)
				}
				buf := e.executors[nodeID].Run(parents, slice)
				cachedOutputs[node.ID] = buf
				delete(needed, nodeID)

				// save it unless it is video
				if node.Type == VideoType {
					continue
				}

				// if no outputs series, create one to store the exec outputs
				vn := GetOrCreateVNode(node, vector)
				if vn.Series == nil {
					db.Transaction(func(tx Tx) {
						var seriesID *int
						tx.QueryRow("SELECT series_id FROM vnodes WHERE id = ?", vn.ID).Scan(&seriesID)
						if seriesID != nil {
							vn.SeriesID = seriesID
							return
						}
						name := fmt.Sprintf("exec-%v-%d", vn.Node.Name, vn.ID)
						res := tx.Exec(
							"INSERT INTO series (timeline_id, name, type, data_type, src_vector, node_id) VALUES (?, ?, 'outputs', ?, ?, ?)",
							vector[0].Timeline.ID, name, vn.Node.Type, Vector(vector).String(), vn.Node.ID,
						)
						vn.SeriesID = new(int)
						*vn.SeriesID = res.LastInsertId()
						tx.Exec("UPDATE vnodes SET series_id = ? WHERE id = ?", vn.SeriesID, vn.ID)
					})
					vn.Series = GetSeries(*vn.SeriesID)
				}

				// persist the outputs
				rd := buf.Reader()
				go func() {
					start := time.Now()
					data, err := rd.Read(slice.Length())
					if err != nil {
						return
					}
					vn.Series.WriteItem(slice, data, rd.Freq())
					e.stats[vn.Node.ID] = NodeStats{
						samples: append(e.stats[vn.Node.ID].samples, time.Now().Sub(start)),
					}
				}()
			}
		}
	}

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		// if a selector is provided, first check it
		if e.query.Selector != nil {
			run([]*Node{e.query.Selector})
			rd := cachedOutputs[e.query.Selector.ID].Reader()
			data, err := rd.Read(slice.Length())
			if err != nil {
				log.Printf("[exec %s %v] error computing selector: %v", e.query.Name, slice, err)
				callback(nil, err)
				return
			}
			if data.IsEmpty() {
				callback(nil, fmt.Errorf("selector reject"))
				return
			}
		}

		run(e.query.FlatOutputs())
		callback(collectOutputs(), nil)

		// for QueryExecutor.Wait support
		for _, buf := range cachedInputs {
			buf.Wait()
		}
		for _, buf := range cachedOutputs {
			buf.Wait()
		}
	}()
}

func (e *QueryExecutor) Close() {
	for _, nodeExec := range e.executors {
		nodeExec.Close()
	}
}

// close after the current outputs finish
func (e *QueryExecutor) Wait() {
	e.wg.Wait()
	e.Close()
}

func init() {
	http.HandleFunc("/queries/nodes", func(w http.ResponseWriter, r *http.Request) {
		// list nodes for a query, or create new node
		r.ParseForm()
		queryID := ParseInt(r.Form.Get("query_id"))
		query := GetQuery(queryID)

		if r.Method == "GET" {
			JsonResponse(w, ListNodesByQuery(query))
			return
		} else if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		name := r.PostForm.Get("name")
		parents := r.PostForm.Get("parents")
		t := r.PostForm.Get("type")
		ext := r.PostForm.Get("ext")
		db.Exec(
			"INSERT INTO nodes (name, parents, type, ext, code, query_id) VALUES (?, ?, ?, ?, '', ?)",
			name, parents, t, ext, queryID,
		)
	})

	http.HandleFunc("/queries/node", func(w http.ResponseWriter, r *http.Request) {
		// get or update a node
		r.ParseForm()
		nodeID := ParseInt(r.Form.Get("id"))
		node := GetNode(nodeID)
		if node == nil {
			w.WriteHeader(404)
			return
		}
		if r.Method == "GET" {
			JsonResponse(w, node)
			return
		} else if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		if r.PostForm["code"] != nil {
			db.Exec("UPDATE nodes SET code = ? WHERE id = ?", r.PostForm.Get("code"), node.ID)
		}
		if r.PostForm["parents"] != nil {
			db.Exec("UPDATE nodes SET parents = ? WHERE id = ?", r.PostForm.Get("parents"), node.ID)
		}
		node.OnChange()
		tasks.queryUpdated(node.QueryID)
	})

	http.HandleFunc("/queries/node/remove", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		r.ParseForm()
		nodeID := ParseInt(r.PostForm.Get("id"))
		node := GetNode(nodeID)
		if node == nil {
			w.WriteHeader(404)
			return
		}
		tasks.queryUpdated(node.QueryID)
		node.Remove()
	})

	http.HandleFunc("/queries", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			JsonResponse(w, ListQueries())
			return
		} else if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		r.ParseForm()
		name := r.PostForm.Get("name")
		res := db.Exec("INSERT INTO queries (name) VALUES (?)", name)
		query := GetQuery(res.LastInsertId())
		JsonResponse(w, query)
	})

	http.HandleFunc("/queries/query", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		query := GetQuery(ParseInt(r.Form.Get("query_id")))
		if query == nil {
			http.Error(w, "no such query", 404)
			return
		}
		if r.Method == "GET" {
			query.Load()
			JsonResponse(w, query)
		} else if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		r.ParseForm()
		if r.PostForm["outputs"] != nil {
			db.Exec("UPDATE queries SET outputs = ? WHERE id = ?", r.PostForm.Get("outputs"), query.ID)
		}
		tasks.queryUpdated(query.ID)
	})

	http.HandleFunc("/queries/render-meta", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		type Request struct {
			ID int
			Meta QueryRenderMeta
		}
		var request Request
		if err := JsonRequest(w, r, &request); err != nil {
			return
		}
		db.Exec(
				"UPDATE queries SET render_meta = ? WHERE id = ?",
				string(JsonMarshal(request.Meta)), request.ID,
		)
	})

	SetupFuncs = append(SetupFuncs, func(server *socketio.Server) {
		var mu sync.Mutex
		taskContexts := make(map[string]*TaskContext)

		type ExecRequest struct {
			Vector string
			QueryID int
			Mode string // "random" or "sequential"
			StartSlice Slice
			Count int
			Continue bool
		}

		server.OnConnect("/exec", func(s socketio.Conn) error {
			return nil
		})

		server.OnError("/exec", func (s socketio.Conn, err error) {
			log.Printf("[socket.io] error on client %v: %v", s.ID(), err)
		})

		server.OnEvent("/exec", "exec", func(s socketio.Conn, request ExecRequest) {
			query := GetQuery(request.QueryID)
			if query == nil {
				s.Emit("error", "no such query")
				return
			}
			vector := ParseVector(request.Vector)
			var sampler func() *Slice
			if request.Mode == "random" {
				sampler = func() *Slice {
					slice := vector[0].Timeline.Uniform(VisualizeMaxFrames)
					return &slice
				}
			} else if request.Mode == "sequential" {
				segment := GetSegment(request.StartSlice.Segment.ID)
				if segment == nil || segment.Timeline.ID != vector[0].Timeline.ID {
					s.Emit("error", "invalid segment for sequential request")
					return
				}
				curIdx := request.StartSlice.Start
				sampler = func() *Slice {
					frameRange := [2]int{curIdx, curIdx+VisualizeMaxFrames}
					if frameRange[1] > segment.Frames {
						return nil
					}
					curIdx += VisualizeMaxFrames
					return &Slice{
						Segment: *segment,
						Start: frameRange[0],
						End: frameRange[1],
					}
				}
			}

			// try to reuse existing task context
			mu.Lock()
			defer mu.Unlock()
			ok := func() bool {
				if !request.Continue {
					return false
				}
				ctx := taskContexts[s.ID()]
				if ctx == nil {
					return false
				}
				if ctx.query.ID != query.ID || Vector(ctx.vector).String() != vector.String() {
					return false
				}
				log.Printf("[exec (%s) %v] reuse existing task context with %d new outputs", query.Name, vector, request.Count)
				ctx.Get(request.Count)
				return true
			}()
			if ok {
				return
			}

			log.Printf("[exec (%s) %v] beginning test for client %v", query.Name, vector, s.ID())
			renderVectors := query.GetOutputVectors(vector)
			ctx := NewTaskContext(query, vector, sampler, request.Count, func(slice Slice, outputs [][]DataReader, err error) {
				if err != nil {
					s.Emit("exec-reject")
					return
				}

				cacheID := uuid.New().String()
				r := RenderVideo(slice, outputs, RenderOpts{ProgressCallback: func(percent int) {
					type ProgressResponse struct {
						UUID string
						Percent int
					}
					s.Emit("exec-progress", ProgressResponse{cacheID, percent})
				}})
				cache.Put(cacheID, r)
				log.Printf("[exec (%s) %v] test: cached renderer with %d frames, uuid=%s", query.Name, slice, slice.Length(), cacheID)
				var t DataType = VideoType
				if len(outputs[0]) >= 2 {
					t = outputs[0][1].Type()
				}
				s.Emit("exec-result", VisualizeResponse{
					PreviewURL: fmt.Sprintf("/cache/preview?id=%s&type=jpeg", cacheID),
					URL: fmt.Sprintf("/cache/view?id=%s", cacheID),
					UUID: cacheID,
					Slice: slice,
					Type: t,
					Vectors: renderVectors,
				})
			})
			ctx.Get(request.Count)
			if taskContexts[s.ID()] != nil {
				taskContexts[s.ID()].Close()
			}
			taskContexts[s.ID()] = ctx
		})

		server.OnDisconnect("/exec", func(s socketio.Conn, e string) {
			mu.Lock()
			if taskContexts[s.ID()] != nil {
				taskContexts[s.ID()].Close()
				taskContexts[s.ID()] = nil
			}
			mu.Unlock()
		})
	})
}
