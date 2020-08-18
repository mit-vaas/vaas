package builtins

import (
	"../vaas"

	"encoding/json"
	"fmt"
)

type ResampleConfig struct {
	// target frequency of output buffer
	// (if input buffer freq = 2, and our freq = 4, then we downsample 2x from the input)
	Freq int
}

type Resample struct {
	node vaas.Node
	cfg ResampleConfig
	stats *vaas.StatsHolder
}

func NewResample(node vaas.Node) vaas.Executor {
	var cfg ResampleConfig
	err := json.Unmarshal([]byte(node.Code), &cfg)
	if err != nil {
		return vaas.ErrorExecutor{node.DataType, fmt.Errorf("error decoding node configuration: %v", err)}
	}
	if cfg.Freq == 0 {
		return vaas.ErrorExecutor{node.DataType, fmt.Errorf("resample node is not configured or misconfigured (freq=0)", err)}
	}
	return Resample{
		node: node,
		cfg: cfg,
		stats: new(vaas.StatsHolder),
	}
}

func (m Resample) Run(ctx vaas.ExecContext) vaas.DataBuffer {
	parents, err := GetParents(ctx, m.node)
	if err != nil {
		return vaas.GetErrorBuffer(m.node.DataType, fmt.Errorf("resample error reading parents: %v", err))
	}
	if len(parents) != 1 {
		panic(fmt.Errorf("resample takes one parent"))
	}
	expectedOutputLength := (ctx.Slice.Length() + m.cfg.Freq-1) / m.cfg.Freq
	parent := parents[0]

	// push-down the operation if possible
	if vbufReader, ok := parent.(*vaas.VideoBufferReader); ok {
		vbufReader.Resample(m.cfg.Freq / vbufReader.Freq())
		vbufReader.SetLength(expectedOutputLength)
		return vbufReader
	}

	buf := vaas.NewSimpleBuffer(parent.Type())
	buf.SetMeta(m.cfg.Freq)

	go func() {
		var count int = 0
		err := vaas.ReadMultiple(
			ctx.Slice.Length(), m.cfg.Freq, parents,
			vaas.ReadMultipleOptions{Stats: m.stats},
			func(index int, datas []vaas.Data) error {
				data := datas[0]
				count += data.Length()
				buf.Write(data)
				return nil
			},
		)
		if err != nil {
			buf.Error(err)
			return
		}
		if count != expectedOutputLength {
			panic(fmt.Errorf("expected resample length %d but got %d", expectedOutputLength, count))
		}
		buf.Close()
	}()

	return buf
}

func (m Resample) Close() {}

func (m Resample) Stats() vaas.StatsSample {
	return m.stats.Get()
}

func init() {
	vaas.Executors["resample"] = vaas.ExecutorMeta{
		New: NewResample,
		HandleResample: true,
		Tune: func(node vaas.Node, gtlist []vaas.Data) [][2]string {
			var cfgs [][2]string
			for _, freq := range []int{1, 2, 4, 8, 16} {
				cfg := ResampleConfig{freq}
				desc := fmt.Sprintf("rate %d", freq)
				cfgs = append(cfgs, [2]string{string(vaas.JsonMarshal(cfg)), desc})
			}
			return cfgs
		},
	}
}
