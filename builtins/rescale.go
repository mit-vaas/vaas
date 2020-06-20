package builtins

import (
	"../skyhook"
)

type RescaleConfig struct {
	Width int
	Height int
}

type Rescale struct {
	cfg RescaleConfig
}

func NewRescale(node *skyhook.Node, query *skyhook.Query) skyhook.Executor {
	var cfg RescaleConfig
	skyhook.JsonUnmarshal([]byte(node.Code), &cfg)
	return Rescale{cfg: cfg}
}

func (m Rescale) Run(parents []skyhook.DataReader, slice skyhook.Slice) skyhook.DataBuffer {
	vbufReader := parents[0].(*skyhook.VideoBufferReader)
	vbufReader.Rescale([2]int{m.cfg.Width, m.cfg.Height})
	return vbufReader
}

func (m Rescale) Close() {}

func init() {
	skyhook.Executors["rescale"] = NewRescale
}
