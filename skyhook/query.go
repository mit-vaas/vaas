package skyhook

import (
	"log"
	"net/http"
	"strings"
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

type Node struct {
	ID int
	Name string
	ParentTypes []DataType
	Parents []Parent
	Type string
	DataType DataType
	Code string
	QueryID int

}

const NodeQuery = "SELECT id, name, parent_types, parents, type, data_type, code, query_id FROM nodes"

func nodeListHelper(rows *Rows) []*Node {
	nodes := []*Node{}
	for rows.Next() {
		var node Node
		var parentTypes, parents string
		rows.Scan(&node.ID, &node.Name, &parentTypes, &parents, &node.Type, &node.DataType, &node.Code, &node.QueryID)
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
	return Executors[node.Type](node)
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

func (node *Node) Update(code *string, parents *string) {
	if code != nil {
		db.Exec("UPDATE nodes SET code = ? WHERE id = ?", *code, node.ID)
	}
	if parents != nil {
		db.Exec("UPDATE nodes SET parents = ? WHERE id = ?", *parents, node.ID)
	}
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

// Metadata for rendering query and its nodes on the front-end query editor.
type QueryRenderMeta map[string][2]int

type Query struct {
	ID int
	Name string
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
	sections := strings.Split(query.OutputsStr, ";")
	query.Outputs = make([][]Parent, len(sections))
	for i, section := range sections {
		query.Outputs[i] = ParseParents(section)
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
		t := r.PostForm.Get("type")
		dataType := r.PostForm.Get("data_type")
		db.Exec(
			"INSERT INTO nodes (name, parents, type, data_type, code, query_id, parent_types) VALUES (?, '', ?, ?, '', ?, '')",
			name, t, dataType, queryID,
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

		var code, parents *string
		if r.PostForm["code"] != nil {
			code = new(string)
			*code = r.PostForm.Get("code")
		}
		if r.PostForm["parents"] != nil {
			parents = new(string)
			*parents = r.PostForm.Get("parents")
		}
		node.Update(code, parents)
		allocator.Deallocate(EnvSetID{"query", node.QueryID})
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
		allocator.Deallocate(EnvSetID{"query", node.QueryID})
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
		allocator.Deallocate(EnvSetID{"query", query.ID})
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
		if err := ParseJsonRequest(w, r, &request); err != nil {
			return
		}
		db.Exec(
				"UPDATE queries SET render_meta = ? WHERE id = ?",
				string(JsonMarshal(request.Meta)), request.ID,
		)
	})
}
