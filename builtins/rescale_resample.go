package builtins

import (
	"../vaas"

	"fmt"
)

type RescaleResampleConfig struct {
	// target frequency of output buffer
	// (if input buffer freq = 2, and our freq = 4, then we downsample 2x from the input)
	Freq int

	Width int
	Height int
}

type RescaleResample struct {
	node vaas.Node
	cfg RescaleResampleConfig
}

func NewRescaleResample(node vaas.Node) vaas.Executor {
	var cfg RescaleResampleConfig
	vaas.JsonUnmarshal([]byte(node.Code), &cfg)
	if cfg.Freq == 0 {
		cfg.Freq = 1
	}
	return RescaleResample{
		node: node,
		cfg: cfg,
	}
}

func (m RescaleResample) Run(ctx vaas.ExecContext) vaas.DataBuffer {
	parents, err := GetParents(ctx, m.node)
	if err != nil {
		return vaas.GetErrorBuffer(m.node.DataType, fmt.Errorf("rescale-resample error reading parents: %v", err))
	}
	if len(parents) != 1 {
		panic(fmt.Errorf("rescale-resample takes one parent"))
	}
	parent := parents[0]
	var targetFreq int
	if m.cfg.Freq == 0 {
		targetFreq = parent.Freq()
	} else {
		targetFreq = m.cfg.Freq
	}
	expectedLength := (ctx.Slice.Length() + targetFreq-1) / targetFreq

	// push-down the operation if possible
	if vbufReader, ok := parent.(*vaas.VideoBufferReader); ok {
		vbufReader.Resample(targetFreq / vbufReader.Freq())
		if m.cfg.Width != 0 && m.cfg.Height != 0 {
			vbufReader.Rescale([2]int{m.cfg.Width, m.cfg.Height})
		}
		vbufReader.SetLength(expectedLength)
		return vbufReader
	}

	buf := vaas.NewSimpleBuffer(parent.Type())
	buf.SetMeta(targetFreq)

	go func() {
		var count int = 0
		err := vaas.ReadMultiple(ctx.Slice.Length(), targetFreq, parents, func(index int, datas []vaas.Data) error {
			data := datas[0]
			count += data.Length()
			buf.Write(data)
			return nil
		})
		if err != nil {
			buf.Error(err)
			return
		}
		if count != expectedLength {
			panic(fmt.Errorf("expected resample length %d but got %d", expectedLength, count))
		}
		buf.Close()
	}()

	return buf
}

func (m RescaleResample) Close() {}

func init() {
	vaas.Executors["rescale-resample"] = vaas.ExecutorMeta{New: NewRescaleResample}
}
