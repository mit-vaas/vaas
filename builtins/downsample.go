package builtins

import (
	"../vaas"

	"encoding/json"
	"fmt"
)

type DownsampleConfig struct {
	// target frequency of output buffer
	// (if input buffer freq = 2, and our freq = 4, then we downsample 2x from the input)
	Freq int
}

type Downsample struct {
	node vaas.Node
	cfg DownsampleConfig
	stats *vaas.StatsHolder
}

func NewDownsample(node vaas.Node) vaas.Executor {
	var cfg DownsampleConfig
	err := json.Unmarshal([]byte(node.Code), &cfg)
	if err != nil {
		return vaas.ErrorExecutor{node.DataType, fmt.Errorf("error decoding node configuration: %v", err)}
	}
	return Downsample{
		node: node,
		cfg: cfg,
		stats: new(vaas.StatsHolder),
	}
}

func (m Downsample) Run(ctx vaas.ExecContext) vaas.DataBuffer {
	parents, err := GetParents(ctx, m.node)
	if err != nil {
		return vaas.GetErrorBuffer(m.node.DataType, fmt.Errorf("downsample error reading parents: %v", err))
	}
	if len(parents) != 1 {
		panic(fmt.Errorf("downsample takes one parent"))
	}
	parent := parents[0]
	expectedLength := (ctx.Slice.Length() + m.cfg.Freq-1) / m.cfg.Freq

	// push-down the re-sample if possible
	if vbufReader, ok := parent.(*vaas.VideoBufferReader); ok {
		vbufReader.Resample(m.cfg.Freq / vbufReader.Freq())
		vbufReader.SetLength(expectedLength)
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
		if count != expectedLength {
			panic(fmt.Errorf("expected downsample length %d but got %d", expectedLength, count))
		}
		buf.Close()
	}()

	return buf
}

func (m Downsample) Close() {}

func (m Downsample) Stats() vaas.StatsSample {
	return m.stats.Get()
}

func init() {
	vaas.Executors["downsample"] = vaas.ExecutorMeta{New: NewDownsample}
}
