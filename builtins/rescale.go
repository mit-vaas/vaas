package builtins

import (
	"../skyhook"
	"fmt"
)

type RescaleConfig struct {
	Width int
	Height int
}

type Rescale struct {
	node *skyhook.Node
	cfg RescaleConfig
}

func NewRescale(node *skyhook.Node) skyhook.Executor {
	var cfg RescaleConfig
	skyhook.JsonUnmarshal([]byte(node.Code), &cfg)
	return Rescale{
		node: node,
		cfg: cfg,
	}
}

func (m Rescale) Run(ctx skyhook.ExecContext) skyhook.DataBuffer {
	parents, err := GetParents(ctx, m.node)
	if err != nil {
		return skyhook.GetErrorBuffer(m.node.DataType, fmt.Errorf("rescale error reading parents: %v", err))
	}
	vbufReader := parents[0].(*skyhook.VideoBufferReader)
	vbufReader.Rescale([2]int{m.cfg.Width, m.cfg.Height})
	return vbufReader
}

func (m Rescale) Close() {}

func init() {
	skyhook.Executors["rescale"] = NewRescale
}
