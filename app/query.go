package app

import (
	"../vaas"

	"fmt"
	"log"
	"net/http"
	"strings"
)

type DBNode struct{vaas.Node}
type DBVNode struct{
	vaas.VNode
	loaded bool
	NodeID int
	VectorStr string
	SeriesID *int
}
type DBQuery struct{vaas.Query}

// Several systems want to listen for when the query changes.
// So we have a general interface for these listeners here.
var QueryChangeListeners []func(*DBQuery)
func OnQueryChanged(query *DBQuery) {
	for _, f := range QueryChangeListeners {
		f(query)
	}
}

const NodeQuery = "SELECT id, name, parent_types, parents, type, data_type, code, query_id FROM nodes"

func nodeListHelper(rows *Rows) []*DBNode {
	nodes := []*DBNode{}
	for rows.Next() {
		var node DBNode
		var parentTypes, parents string
		rows.Scan(&node.ID, &node.Name, &parentTypes, &parents, &node.Type, &node.DataType, &node.Code, &node.QueryID)
		if parentTypes != "" {
			for _, dt := range strings.Split(parentTypes, ",") {
				node.ParentTypes = append(node.ParentTypes, vaas.DataType(dt))
			}
		}
		node.Parents = vaas.ParseParents(parents)
		nodes = append(nodes, &node)
	}
	return nodes
}

func ListNodesByQuery(query *DBQuery) []*DBNode {
	rows := db.Query(NodeQuery + " WHERE query_id = ?", query.ID)
	return nodeListHelper(rows)
}

func GetNode(id int) *DBNode {
	rows := db.Query(NodeQuery + " WHERE id = ?", id)
	nodes := nodeListHelper(rows)
	if len(nodes) == 1 {
		return nodes[0]
	} else {
		return nil
	}
}

func (node *DBNode) ListVNodes() []*DBVNode {
	return ListVNodesWithNode(node)
}

func (node *DBNode) Update(code *string, parents *string) {
	if code != nil {
		db.Exec("UPDATE nodes SET code = ? WHERE id = ?", *code, node.ID)
	}
	if parents != nil {
		db.Exec("UPDATE nodes SET parents = ? WHERE id = ?", *parents, node.ID)
	}
	node.OnChange()
}

func (node DBNode) encodeParents() string {
	var parts []string
	for _, parent := range node.Parents {
		if parent.Type == vaas.NodeParent {
			parts = append(parts, fmt.Sprintf("%s%d", vaas.NodeParent, parent.NodeID))
		} else if parent.Type == vaas.SeriesParent {
			parts = append(parts, fmt.Sprintf("%s%d", vaas.SeriesParent, parent.SeriesIdx))
		}
	}
	return strings.Join(parts, ",")
}

func (node DBNode) encodeParentTypes() string {
	var parts []string
	for _, dt := range node.ParentTypes {
		parts = append(parts, string(dt))
	}
	return strings.Join(parts, ",")
}

func (node DBNode) Save() {
	db.Exec(
		"UPDATE nodes SET name = ?, parent_types = ?, parents = ?, type = ?, data_type = ?, code = ? WHERE id = ?",
		node.Name, node.encodeParentTypes(), node.encodeParents(), node.Type, node.DataType, node.Code, node.ID,
	)
	node.OnChange()
}

func (node *DBNode) OnChange() {
	// delete all saved labels at the vnodes for this node
	// and recursively delete for vnodes that depend on that vnode

	query := GetQuery(node.QueryID)
	query.Load()

	affectedNodes := map[int]*DBNode{node.ID: node}
	queue := []*DBNode{node}
	for len(queue) > 0 {
		head := queue[0]
		queue = queue[1:]
		// find all children of head
		for _, node := range head.GetChildren(query.Nodes) {
			if affectedNodes[node.ID] != nil {
				continue
			}
			dbnode := &DBNode{Node: node}
			affectedNodes[node.ID] = dbnode
			queue = append(queue, dbnode)
		}
	}

	for _, node := range affectedNodes {
		for _, vn := range node.ListVNodes() {
			vn.Clear()
		}
	}
}

const VNodeQuery = "SELECT id, node_id, vector, series_id FROM vnodes"

func vnodeListHelper(rows *Rows) []*DBVNode {
	var vnodes []*DBVNode
	for rows.Next() {
		var vn DBVNode
		rows.Scan(&vn.ID, &vn.NodeID, &vn.VectorStr, &vn.SeriesID)
		vnodes = append(vnodes, &vn)
	}
	return vnodes
}

func GetVNode(node *DBNode, vector []*DBSeries) *DBVNode {
	rows := db.Query(VNodeQuery + " WHERE vector = ? AND node_id = ?", Vector(vector).String(), node.ID)
	vnodes := vnodeListHelper(rows)
	if len(vnodes) == 1 {
		vnodes[0].Load()
		return vnodes[0]
	} else {
		return nil
	}
}

func GetOrCreateVNode(node *DBNode, vector []*DBSeries) *DBVNode {
	vn := GetVNode(node, vector)
	if vn != nil {
		return vn
	}
	db.Exec("INSERT OR REPLACE INTO vnodes (node_id, vector) VALUES (?, ?)", node.ID, Vector(vector).String())
	return GetVNode(node, vector)
}

func ListVNodesWithNode(node *DBNode) []*DBVNode {
	rows := db.Query(VNodeQuery + " WHERE node_id = ?", node.ID)
	return vnodeListHelper(rows)
}

func (vn *DBVNode) Load() {
	if vn.loaded {
		return
	}
	for _, series := range ParseVector(vn.VectorStr) {
		vn.Vector = append(vn.Vector, series.Series)
	}
	vn.Node = GetNode(vn.NodeID).Node
	if vn.SeriesID != nil {
		vn.Series = new(vaas.Series)
		*vn.Series = GetSeries(*vn.SeriesID).Series
	}
	vn.loaded = true
}

// clear saved labels at a node
func (vn *DBVNode) Clear() {
	vn.Load()
	if vn.Series != nil {
		log.Printf("[exec (%s)] clearing outputs series %s", vn.Node.Name, vn.Series.Name)
		DBSeries{Series: *vn.Series}.Clear()
	}
}

const QueryQuery = "SELECT id, name, outputs, selector, render_meta FROM queries"

func queryListHelper(rows *Rows) []*DBQuery {
	queries := []*DBQuery{}
	for rows.Next() {
		var query DBQuery
		var renderMeta string
		rows.Scan(&query.ID, &query.Name, &query.OutputsStr, &query.SelectorID, &renderMeta)
		if renderMeta != "" {
			vaas.JsonUnmarshal([]byte(renderMeta), &query.RenderMeta)
		}
		queries = append(queries, &query)
	}
	return queries
}

func GetQuery(queryID int) *DBQuery {
	rows := db.Query(QueryQuery + " WHERE id = ?", queryID)
	queries := queryListHelper(rows)
	if len(queries) == 1 {
		return queries[0]
	} else {
		return nil
	}
}

func ListQueries() []*DBQuery {
	rows := db.Query(QueryQuery)
	return queryListHelper(rows)
}

func (query *DBQuery) FlatOutputs() []*DBNode {
	query.Load()
	var flat []*DBNode
	for _, outputs := range query.Outputs {
		for _, output := range outputs {
			if output.Type != vaas.NodeParent {
				continue
			}
			flat = append(flat, &DBNode{Node: *query.Nodes[output.NodeID]})
		}
	}
	return flat
}

func (query *DBQuery) Load() {
	if query.Nodes != nil {
		return
	}

	query.Nodes = make(map[int]*vaas.Node)
	for _, node := range ListNodesByQuery(query) {
		n := node.Node
		query.Nodes[node.ID] = &n
	}

	// load output and selector
	sections := strings.Split(query.OutputsStr, ";")
	query.Outputs = make([][]vaas.Parent, len(sections))
	for i, section := range sections {
		query.Outputs[i] = vaas.ParseParents(section)
	}
	if query.SelectorID != nil {
		query.Selector = query.Nodes[*query.SelectorID]
	}
}

func (query *DBQuery) GetOutputVectors(inputs []*DBSeries) [][]*DBSeries {
	query.Load()
	outputs := make([][]*DBSeries, len(query.Outputs))
	for i, l := range query.Outputs {
		for _, output := range l {
			if output.Type == vaas.NodeParent {
				node := query.Nodes[output.NodeID]
				vn := GetOrCreateVNode(&DBNode{Node: *node}, inputs)
				if vn.Series == nil {
					outputs[i] = append(outputs[i], nil)
				} else {
					outputs[i] = append(outputs[i], &DBSeries{Series: *vn.Series})
				}
			} else if output.Type == vaas.SeriesParent {
				outputs[i] = append(outputs[i], inputs[output.SeriesIdx])
			}
		}
	}
	return outputs
}

func (query *DBQuery) AddNode(name string, t string, dataType vaas.DataType) *DBNode {
	res := db.Exec(
		"INSERT INTO nodes (name, parents, type, data_type, code, query_id, parent_types) VALUES (?, '', ?, ?, '', ?, '')",
		name, t, dataType, query.ID,
	)
	node := GetNode(res.LastInsertId())
	if query.Nodes != nil {
		query.Nodes[node.ID] = &node.Node
	}
	return node
}

func (query *DBQuery) RemoveNode(node *DBNode) {
	query.Load()
	node.OnChange()
	db.Exec("DELETE FROM nodes WHERE id = ?", node.ID)
	for _, n := range query.Nodes {
		idx := -1
		for i, parent := range n.Parents {
			if parent.Type != vaas.NodeParent || parent.NodeID != node.ID {
				continue
			}
			idx = i
			break
		}
		if idx == -1 {
			continue
		}
		copy(n.Parents[idx:], n.Parents[idx+1:])
		n.Parents = n.Parents[0:len(n.Parents)-1]
		DBNode{Node: *n}.Save()
	}
}

func init() {
	http.HandleFunc("/queries/nodes", func(w http.ResponseWriter, r *http.Request) {
		// list nodes for a query, or create new node
		r.ParseForm()
		queryID := vaas.ParseInt(r.Form.Get("query_id"))
		query := GetQuery(queryID)

		if r.Method == "GET" {
			vaas.JsonResponse(w, ListNodesByQuery(query))
			return
		} else if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		name := r.PostForm.Get("name")
		t := r.PostForm.Get("type")
		dataType := r.PostForm.Get("data_type")
		query.AddNode(name, t, vaas.DataType(dataType))
	})

	http.HandleFunc("/queries/node", func(w http.ResponseWriter, r *http.Request) {
		// get or update a node
		r.ParseForm()
		nodeID := vaas.ParseInt(r.Form.Get("id"))
		node := GetNode(nodeID)
		if node == nil {
			w.WriteHeader(404)
			return
		}
		if r.Method == "GET" {
			vaas.JsonResponse(w, node)
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
		OnQueryChanged(GetQuery(node.QueryID))
	})

	http.HandleFunc("/queries/node/remove", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		r.ParseForm()
		nodeID := vaas.ParseInt(r.PostForm.Get("id"))
		node := GetNode(nodeID)
		if node == nil {
			w.WriteHeader(404)
			return
		}
		query := GetQuery(node.QueryID)
		query.RemoveNode(node)
		OnQueryChanged(query)
	})

	http.HandleFunc("/queries", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			vaas.JsonResponse(w, ListQueries())
			return
		} else if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		r.ParseForm()
		name := r.PostForm.Get("name")
		res := db.Exec("INSERT INTO queries (name) VALUES (?)", name)
		query := GetQuery(res.LastInsertId())
		vaas.JsonResponse(w, query)
	})

	http.HandleFunc("/queries/query", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		query := GetQuery(vaas.ParseInt(r.Form.Get("query_id")))
		if query == nil {
			http.Error(w, "no such query", 404)
			return
		}
		if r.Method == "GET" {
			query.Load()
			vaas.JsonResponse(w, query)
			return
		} else if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		r.ParseForm()
		if r.PostForm["outputs"] != nil {
			db.Exec("UPDATE queries SET outputs = ? WHERE id = ?", r.PostForm.Get("outputs"), query.ID)
		}
		if r.PostForm["selector"] != nil {
			selector := r.PostForm.Get("selector")
			if selector == "" {
				db.Exec("UPDATE queries SET selector = NULL WHERE id = ?", query.ID)
			} else {
				db.Exec("UPDATE queries SET selector = ? WHERE id = ?", selector, query.ID)
			}
		}
		OnQueryChanged(query)
	})

	http.HandleFunc("/queries/render-meta", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		type Request struct {
			ID int
			Meta vaas.QueryRenderMeta
		}
		var request Request
		if err := vaas.ParseJsonRequest(w, r, &request); err != nil {
			return
		}
		db.Exec(
				"UPDATE queries SET render_meta = ? WHERE id = ?",
				string(vaas.JsonMarshal(request.Meta)), request.ID,
		)
	})
}
