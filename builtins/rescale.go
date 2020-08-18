package builtins

import (
	"../vaas"

	"encoding/json"
	"fmt"
)

type RescaleConfig struct {
	Width int
	Height int
}

type Rescale struct {
	node vaas.Node
	cfg RescaleConfig
	stats *vaas.StatsHolder
}

func NewRescale(node vaas.Node) vaas.Executor {
	var cfg RescaleConfig
	err := json.Unmarshal([]byte(node.Code), &cfg)
	if err != nil {
		return vaas.ErrorExecutor{node.DataType, fmt.Errorf("error decoding node configuration: %v", err)}
	}
	if cfg.Width == 0 || cfg.Height == 0 {
		return vaas.ErrorExecutor{node.DataType, fmt.Errorf("rescale node is not configured or misconfigured (width or height is 0)", err)}
	}
	return Rescale{
		node: node,
		cfg: cfg,
		stats: new(vaas.StatsHolder),
	}
}

func (m Rescale) Run(ctx vaas.ExecContext) vaas.DataBuffer {
	parents, err := GetParents(ctx, m.node)
	if err != nil {
		return vaas.GetErrorBuffer(m.node.DataType, fmt.Errorf("rescale error reading parents: %v", err))
	}
	if len(parents) != 1 {
		panic(fmt.Errorf("rescale takes one parent"))
	}
	parent := parents[0]

	// push-down the operation if possible
	if vbufReader, ok := parent.(*vaas.VideoBufferReader); ok {
		vbufReader.Rescale([2]int{m.cfg.Width, m.cfg.Height})
		return vbufReader
	}

	return vaas.GetErrorBuffer(m.node.DataType, fmt.Errorf("rescale only supports parents that emit VideoBufferReader"))
}

func (m Rescale) Close() {}

func (m Rescale) Stats() vaas.StatsSample {
	return m.stats.Get()
}

func init() {
	vaas.Executors["rescale"] = vaas.ExecutorMeta{
		New: NewRescale,
		HandleRescale: true,
		Tune: func(node vaas.Node, gtlist []vaas.Data) [][2]string {
			var cfgs [][2]string
			for _, dims := range [][2]int{{1280, 720}, {960, 540}, {640, 360}, {480, 270}} {
				cfg := RescaleConfig{dims[0], dims[1]}
				desc := fmt.Sprintf("%dx%d", dims[0], dims[1])
				cfgs = append(cfgs, [2]string{string(vaas.JsonMarshal(cfg)), desc})
			}
			return cfgs
		},
	}
}
