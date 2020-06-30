package vaas

import (
	"bytes"
	"fmt"
	"net/http"
)

type ExecContext struct {
	// all nodes in the query
	Nodes map[int]*Node

	// the container that each node is assigned to
	Containers map[int]Container

	// the node we actually want to run
	Node Node

	// UUID for this particular execution of the query
	UUID string

	// nodes where we already have label
	Items map[int]*Item

	// input items from vector series
	Inputs []Item

	// other info
	Vector []Series
	Slice Slice
}

func (context ExecContext) GetReader(node Node) (DataReader, error) {
	item := context.Items[node.ID]
	if item != nil {
		return item.Load(context.Slice).Reader(), nil
	}
	container := context.Containers[node.ID]
	path := fmt.Sprintf("/query/start?node_id=%d", node.ID)
	resp, err := http.Post(
		container.BaseURL + path,
		"application/json", bytes.NewBuffer(JsonMarshal(context)),
	)
	if err != nil {
		return nil, fmt.Errorf("error performing HTTP request: %v", err)
	} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP error %d", resp.StatusCode)
	}
	var buf DataBuffer
	if node.DataType == VideoType {
		buf = VideoBufferFromReader(resp.Body)
	} else {
		sbuf := NewSimpleBuffer(node.DataType)
		go func() {
			sbuf.FromReader(resp.Body)
			resp.Body.Close()
		}()
		buf = sbuf
	}

	return buf.Reader(), nil
}

type Executor interface {
	Run(context ExecContext) DataBuffer
	Close()
}

var Executors = map[string]func(Node) Executor{}
