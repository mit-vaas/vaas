package vaas

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
)

type ExecOptions struct {
	PersistVideo bool

	// during tuning, we ignore any input items, and don't persist any outputs
	Tuning bool

	// if true, don't persist any outputs
	// handled by the container
	NoPersist bool

	// if true, don't use any persisted outputs
	// handled by the coordinator
	IgnoreItems bool

	// override selector
	Selector *Node
	NoSelector bool

	// override outputs
	Outputs [][]Parent
}

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

	Opts ExecOptions
}

func (context ExecContext) GetBuffer(node Node) (DataBuffer, error) {
	item := context.Items[node.ID]
	if item != nil {
		return item.Load(context.Slice), nil
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

	return buf, nil
}

func (context ExecContext) GetReader(node Node) (DataReader, error) {
	buf, err := context.GetBuffer(node)
	if err != nil {
		return nil, err
	}
	return buf.Reader(), nil
}

// Notifies containers that they can release the buffers for this context.
// This can be called as soon as the last GetReader call is made.
// Releasing will remove pointer to buffer but existing readers can finish reading.
func (context ExecContext) Release() {
	// release each unique container
	seen := make(map[string]bool)
	for _, container := range context.Containers {
		if seen[container.UUID] {
			continue
		}
		seen[container.UUID] = true
		resp, err := http.Post(container.BaseURL + "/query/finish?uuid=" + context.UUID, "", nil)
		if err != nil {
			// could be due to container de-allocation
			log.Printf("[context] warning: error releasing container %s (%s): %v", container.BaseURL, container.UUID, err)
			continue
		}
		resp.Body.Close()
	}
}

type Executor interface {
	Run(context ExecContext) DataBuffer
	Close()
}

type ErrorExecutor struct {
	DataType DataType
	Error error
}
func (e ErrorExecutor) Run(context ExecContext) DataBuffer { return GetErrorBuffer(e.DataType, e.Error) }
func (e ErrorExecutor) Close() {}

type ExecutorMeta struct {
	New func(Node) Executor

	// only set for non-default environments
	Environment *Environment

	// whether the system should erase items when inputs have been
	// re-scaled and/or re-sampled with no other modifications
	// e.g. an object detector can set these true if it will take control of deciding
	// to re-compute detections (if the input resolution has increased) or reuse old
	// outputs at reduced resolutions (if the opposite)
	HandleRescale bool
	HandleResample bool

	// Returns list of configs that the node should be tested with.
	// Tuples are (config, short description of the config).
	// During tuning, node.Code is set to one of these configs.
	Tune func(node Node, gtlist []Data) [][2]string
}

var Executors = map[string]ExecutorMeta{}
