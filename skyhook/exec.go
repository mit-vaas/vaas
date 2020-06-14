package skyhook

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ParentType string
const (
	NodeParent ParentType = "o"
	VideoParent = "v"
)
type Parent struct {
	Type ParentType
	ID int
	Node *Node
}

func (parent Parent) DataType() DataType {
	if parent.Type == NodeParent {
		return parent.Node.Type
	} else if parent.Type == VideoParent {
		return VideoType
	}
	panic(fmt.Errorf("bad parent type %v", parent.Type))
}

type Executor interface {
	Run(parents []*LabelBuffer, slice ClipSlice) *LabelBuffer
	Close()
}

type Node struct {
	ID int
	Name string
	ParentSpecs []Parent // parents without *Node filled in
	Type DataType
	Ext string
	Code string

	Parents []Parent
}

const NodeQuery = "SELECT id, name, parents, type, ext, code FROM nodes"

func nodeListHelper(rows *Rows) []*Node {
	nodes := []*Node{}
	for rows.Next() {
		var node Node
		var parents string
		rows.Scan(&node.ID, &node.Name, &parents, &node.Type, &node.Ext, &node.Code)
		for _, ps := range strings.Split(parents, ",") {
			var parent Parent
			parent.Type = ParentType(ps[0:1])
			if parent.Type == NodeParent {
				parent.ID, _ = strconv.Atoi(ps[1:])
			}
			node.ParentSpecs = append(node.ParentSpecs, parent)
		}
		nodes = append(nodes, &node)
	}
	return nodes
}

func ListNodes() []*Node {
	rows := db.Query(NodeQuery)
	return nodeListHelper(rows)
}

func GetNode(id int) *Node {
	rows := db.Query(NodeQuery + " WHERE id = ?", id)
	nodes := nodeListHelper(rows)
	if len(nodes) == 1 {
		nodes[0].Load()
		return nodes[0]
	} else {
		return nil
	}
}

func (node *Node) Load() {
	node.LoadMap(map[int]*Node{})
}

func (node *Node) LoadMap(m map[int]*Node) {
	populate := len(node.Parents) == 0
	m[node.ID] = node
	for _, parent := range node.ParentSpecs {
		if parent.Type == NodeParent {
			if m[parent.ID] == nil {
				GetNode(parent.ID).LoadMap(m)
			}
			parent.Node = m[parent.ID]
		}
		if populate {
			node.Parents = append(node.Parents, parent)
		}
	}
}

func (node *Node) ListVNodes() []*VNode {
	return ListVNodesWithNode(node)
}

// Returns label for this node on the specified clip.
func (node *Node) Exec(query *Query) Executor {
	if node.Ext == "python" {
		return node.pythonExecutor(query)
	} else if node.Ext == "model" {
		//return node.modelExecutor(query)
	}
	panic(fmt.Errorf("exec on node with unknown ext %s", node.Ext))
}
/*
	// parents[i][j] is the output of parent i on slice j
	log.Printf("[exec (%s/%s) %v] begin node testing", query.Name, node.Name, slices)
	parents := make([][]*LabelBuffer, len(node.Parents))
	for parentIdx, parent := range node.Parents {
		if parent.Type == NodeParent {
			parentVN := GetVNode(parent.Node, slices[0].Clip.Video)
			for _, labelBuf := range parentVN.Test(query, slices) {
				parents[parentIdx] = append(parents[parentIdx], labelBuf)
			}
		} else if parent.Type == VideoParent {
			for _, slice := range slices {
				rd := ReadVideo(slice, slice.Clip.Width, slice.Clip.Height)
				buf := NewLabelBuffer(VideoType)
				go buf.FromVideoReader(rd)
				parents[parentIdx] = append(parents[parentIdx], buf)
			}
		}
	}

	if node.Ext == "python" {
		return node.testPython(query, parents, slices)
	} else if node.Ext == "model" {
		return node.testModel(query, parents, slices)
	}
	return nil*/

type VNode struct {
	ID int
	NodeID int
	Video Video
	LabelSetID *int

	Node *Node
	LabelSet *LabelSet
}

const VNodeQuery = "SELECT vn.id, vn.node_id, v.id, v.name, v.ext, vn.ls_id FROM vnodes AS vn, videos AS v WHERE v.id = vn.video_id"

func vnodeListHelper(rows *Rows) []*VNode {
	var vnodes []*VNode
	for rows.Next() {
		var vn VNode
		rows.Scan(&vn.ID, &vn.NodeID, &vn.Video.ID, &vn.Video.Name, &vn.Video.Ext, &vn.LabelSetID)
		vnodes = append(vnodes, &vn)
	}
	return vnodes
}

func GetVNode(node *Node, video Video) *VNode {
	rows := db.Query(VNodeQuery + " AND vn.video_id = ? AND vn.node_id = ?", video.ID, node.ID)
	vnodes := vnodeListHelper(rows)
	if len(vnodes) == 1 {
		vnodes[0].Load()
		return vnodes[0]
	} else {
		return nil
	}
}

func GetOrCreateVNode(node *Node, video Video) *VNode {
	vn := GetVNode(node, video)
	if vn != nil {
		return vn
	}
	db.Exec("INSERT INTO vnodes (node_id, video_id) VALUES (?, ?)", node.ID, video.ID)
	return GetVNode(node, video)
}

func ListVNodesWithNode(node *Node) []*VNode {
	rows := db.Query(VNodeQuery + " AND vn.node_id = ?", node.ID)
	return vnodeListHelper(rows)
}

func (vn *VNode) Load() {
	if vn.Node != nil || vn.LabelSet != nil {
		return
	}
	vn.Node = GetNode(vn.NodeID)
	if vn.LabelSetID != nil {
		vn.LabelSet = GetLabelSet(*vn.LabelSetID)
	}
}

// clear saved labels at a node
func (vn *VNode) Clear() {
	vn.Load()
	if vn.LabelSet != nil {
		log.Printf("[exec (%s)] clearing label set %d", vn.Node.Name, vn.LabelSet.ID)
		vn.LabelSet.Clear()
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

type Query struct {
	ID int
	Name string
	Plan *Plan
	NodeID int

	Node *Node

	Nodes map[int]*Node
}



const QueryQuery = "SELECT q.id, q.name, q.node_id FROM queries AS q"

func queryListHelper(rows *Rows) []*Query {
	var queries []*Query
	for rows.Next() {
		var query Query
		rows.Scan(&query.ID, &query.Name, &query.NodeID)
		queries = append(queries, &query)
	}
	for _, query := range queries {
		query.Node = GetNode(query.NodeID)
	}
	return queries
}

func GetQuery(queryID int) *Query {
	rows := db.Query(QueryQuery + " WHERE q.id = ?", queryID)
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

func (query *Query) Load() {
	if len(query.Nodes) > 0 {
		return
	}
	m := make(map[int]*Node)
	query.Node.LoadMap(m)
	query.Nodes = m
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

func (e *QueryExecutor) Run(parents []*LabelBuffer, slice ClipSlice) *LabelBuffer {
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
	cachedOutputs := make(map[int]*LabelBuffer)

	// (1) load labels that we already have
	loadLabels := func(node *Node) bool {
		if cachedOutputs[node.ID] != nil {
			return true
		}
		vn := GetOrCreateVNode(node, slice.Clip.Video)
		if vn.LabelSet == nil {
			return false
		}
		label := vn.LabelSet.GetBySlice(slice)
		if label == nil {
			return false
		}
		cachedOutputs[node.ID] = label.Load(slice)
		return true
	}
	q := []*Node{e.query.Node}
	seen := make(map[int]bool)
	needed := make(map[int]bool)
	// first pass: load the query output nodes
	haveAllOutputs := true
	for _, node := range q {
		seen[node.ID] = true
		ok := loadLabels(node)
		if !ok {
			haveAllOutputs = false
		}
	}
	if haveAllOutputs {
		return cachedOutputs[e.query.Node.ID]
	}
	// second pass: go down graph to load the rest
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
			} else if seen[parent.Node.ID] {
				continue
			}
			seen[parent.Node.ID] = true
			q = append(q, parent.Node)
		}
	}

	var videoBuffer *LabelBuffer
	collectParents := func(node *Node) []*LabelBuffer {
		parents := []*LabelBuffer{}
		for _, parent := range node.Parents {
			if parent.Type == VideoParent {
				if videoBuffer == nil {
					rd := ReadVideo(slice, slice.Clip.Width, slice.Clip.Height)
					videoBuffer = NewLabelBuffer(VideoType)
					go videoBuffer.FromVideoReader(rd)
				}
				parents = append(parents, videoBuffer)
			} else if parent.Type == NodeParent {
				buf := cachedOutputs[parent.Node.ID]
				if buf == nil {
					return nil
				}
				parents = append(parents, buf)
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
			cachedOutputs[nodeID] = e.executors[nodeID].Run(parents, slice)
			delete(needed, nodeID)

			// if no label set, create one to store the exec outputs
			vn := GetOrCreateVNode(node, slice.Clip.Video)
			if vn.LabelSet == nil {
				db.Transaction(func(tx Tx) {
					var lsID *int
					tx.QueryRow("SELECT ls_id FROM vnodes WHERE id = ?", vn.ID).Scan(&lsID)
					if lsID != nil {
						vn.LabelSetID = lsID
						return
					}
					name := fmt.Sprintf("exec-%v-%d", vn.Node.Name, vn.ID)
					res := tx.Exec(
						"INSERT INTO label_sets (name, unit, src_video, video_id, label_type) VALUES (?, ?, ?, ?, ?)",
						name, 1, vn.Video.ID, vn.Video.ID, vn.Node.Type,
					)
					vn.LabelSetID = new(int)
					*vn.LabelSetID = res.LastInsertId()
					tx.Exec("UPDATE vnodes SET ls_id = ? WHERE id = ?", vn.LabelSetID, vn.ID)
				})
				vn.LabelSet = GetLabelSet(*vn.LabelSetID)
			}

			// persist the outputs
			go func() {
				start := time.Now()
				data, err := cachedOutputs[vn.Node.ID].ReadFull(slice.Length())
				if err != nil {
					return
				}
				vn.LabelSet.AddLabel(slice, data)
				e.stats[vn.Node.ID] = NodeStats{
					samples: append(e.stats[vn.Node.ID].samples, time.Now().Sub(start)),
				}
			}()
		}
	}

	out := cachedOutputs[e.query.Node.ID]

	e.wg.Add(1)
	go func() {
		out.ReadFull(slice.Length())
		e.wg.Done()
	}()

	return out
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
	http.HandleFunc("/nodes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			JsonResponse(w, ListNodes())
			return
		} else if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		r.ParseForm()
		name := r.PostForm.Get("name")
		parents := r.PostForm.Get("parents")
		t := r.PostForm.Get("type")
		ext := r.PostForm.Get("ext")
		db.Exec(
			"INSERT INTO nodes (name, parents, type, ext, code) VALUES (?, ?, ?, ?, '')",
			name, parents, t, ext,
		)
	})
	http.HandleFunc("/node", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		nodeID, _ := strconv.Atoi(r.Form.Get("id"))
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

		code := r.PostForm.Get("code")
		db.Exec("UPDATE nodes SET code = ? WHERE id = ?", code, node.ID)

		// delete all saved labels at the vnodes for this node
		// and recursively delete for vnodes that depend on that vnode
		affectedNodes := map[int]*Node{node.ID: node}
		queue := []*Node{node}
		allNodes := ListNodes()
		for len(queue) > 0 {
			head := queue[0]
			queue = queue[1:]
			// find all children of head
			for _, node := range allNodes {
				if affectedNodes[node.ID] != nil {
					continue
				}
				isChild := false
				for _, parent := range node.ParentSpecs {
					if parent.Type != NodeParent {
						continue
					}
					if parent.ID != head.ID {
						continue
					}
					isChild = true
				}
				if !isChild {
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

		tasks.nodeUpdated(node)
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
		nodeID, _ := strconv.Atoi(r.PostForm.Get("node_id"))
		db.Exec(
			"INSERT INTO queries (name, node_id) VALUES (?, ?)",
			name, nodeID,
		)
	})

	http.HandleFunc("/exec/test", func(w http.ResponseWriter, r *http.Request) {
		type TestRequest struct {
			VideoID int
			QueryID int
			Mode string // "random" or "sequential"
			StartSlice ClipSlice
			Count int
		}
		var request TestRequest
		if err := JsonRequest(w, r, &request); err != nil {
			http.Error(w, fmt.Sprintf("json decode error: %v", err), 400)
			return
		}

		query := GetQuery(request.QueryID)
		if query == nil {
			http.Error(w, "no such query", 404)
			return
		}
		video := GetVideo(request.VideoID)
		if video == nil {
			http.Error(w, "no such video", 404)
			return
		}
		var slices []ClipSlice
		if request.Mode == "random" {
			for i := 0; i < request.Count; i++ {
				slices = append(slices, video.Uniform(VisualizeMaxFrames))
			}
		} else if request.Mode == "sequential" {
			clip := GetClip(request.StartSlice.Clip.ID)
			if clip == nil || clip.Video.ID != video.ID {
				http.Error(w, "invalid clip for sequential request", 404)
				return
			}
			startIdx := request.StartSlice.Start
			for i := 0; i < request.Count; i++ {
				frameRange := [2]int{startIdx+i*VisualizeMaxFrames, startIdx+(i+1)*VisualizeMaxFrames}
				if frameRange[1] > clip.Frames {
					break
				}
				slices = append(slices, ClipSlice{
					Clip: *clip,
					Start: frameRange[0],
					End: frameRange[1],
				})
			}
		}

		log.Printf("[exec (%s) %v] beginning test", query.Name, slices)
		task := Task{
			Query: query,
			Slices: slices,
		}
		labelBufs := tasks.Schedule(task)
		log.Printf("[exec (%s) %v] test: got labelBufs, loading previews", query.Name, slices)
		var response []VisualizeResponse
		for i, buf := range labelBufs {
			pc := CreatePreview(slices[i], buf)
			uuid := cache.Add(pc)
			log.Printf("[exec (%s) %v] test: cached preview with %d frames, uuid=%s", query.Name, slices[i], pc.Slice.Length(), uuid)
			response = append(response, VisualizeResponse{
				PreviewURL: fmt.Sprintf("/cache/preview?id=%s&type=jpeg", uuid),
				URL: fmt.Sprintf("/cache/view?id=%s&type=mp4", uuid),
				Width: slices[i].Clip.Width,
				Height: slices[i].Clip.Height,
				UUID: uuid,
				Slice: slices[i],
			})
		}
		JsonResponse(w, response)
	})

	/*http.HandleFunc("/exec/test2", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		videoID, _ := strconv.Atoi(r.PostForm.Get("video_id"))
		queryID, _ := strconv.Atoi(r.PostForm.Get("query_id"))

		query := GetQuery(queryID)
		if query == nil {
			w.WriteHeader(404)
			return
		}
		video := GetVideo(videoID)
		if video == nil {
			w.WriteHeader(404)
			return
		}
		vn := GetOrCreateVNode(node, *video)
		vn.Load()
		for {
			slice := video.Uniform(VisualizeMaxFrames)
			log.Printf("[exec (%s) %v] beginning test", node.Name, slice)
			labelBuf := query.Test([]ClipSlice{slice})[0]
			if labelBuf == nil {
				w.WriteHeader(404)
				return
			}
			data, err := labelBuf.ReadFull(slice.Length())
			if err != nil {
				log.Printf("[exec (%s) %v] error reading exec output: %v", node.Name, slice, err)
				w.WriteHeader(400)
				return
			} else if data.IsEmpty() {
				continue
			}
			log.Printf("[exec (%s) %v] test: got labelBuf, loading preview", node.Name, slice)
			nbuf := NewLabelBuffer(data.Type)
			nbuf.Write(data)
			pc := CreatePreview(slice, nbuf)
			uuid := cache.Add(pc)
			log.Printf("[exec (%s) %v] test: cached preview with %d frames, uuid=%s", node.Name, slice, pc.Slice.Length(), uuid)
			JsonResponse(w, VisualizeResponse{
				PreviewURL: fmt.Sprintf("/cache/preview?id=%s&type=jpeg", uuid),
				URL: fmt.Sprintf("/cache/view?id=%s&type=mp4", uuid),
				Width: slice.Clip.Width,
				Height: slice.Clip.Height,
				UUID: uuid,
			})
			break
		}
	})*/
}
