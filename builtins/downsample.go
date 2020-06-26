package builtins

import (
	"../skyhook"
	"fmt"
)

type DownsampleConfig struct {
	// target frequency of output buffer
	// (if input buffer freq = 2, and our freq = 4, then we downsample 2x from the input)
	Freq int
}

type Downsample struct {
	node *skyhook.Node
	cfg DownsampleConfig
}

func NewDownsample(node *skyhook.Node) skyhook.Executor {
	var cfg DownsampleConfig
	skyhook.JsonUnmarshal([]byte(node.Code), &cfg)
	return Downsample{
		node: node,
		cfg: cfg,
	}
}

func (m Downsample) Run(ctx skyhook.ExecContext) skyhook.DataBuffer {
	parents, err := GetParents(ctx, m.node)
	if err != nil {
		return skyhook.GetErrorBuffer(m.node.DataType, fmt.Errorf("downsample error reading parents: %v", err))
	}
	if len(parents) != 1 {
		panic(fmt.Errorf("downsample takes one parent"))
	}
	parent := parents[0]
	expectedLength := (ctx.Slice.Length() + m.cfg.Freq-1) / m.cfg.Freq

	// push-down the re-sample if possible
	if vbufReader, ok := parent.(*skyhook.VideoBufferReader); ok {
		vbufReader.Resample(m.cfg.Freq / vbufReader.Freq())
		vbufReader.SetLength(expectedLength)
		return vbufReader
	}

	buf := skyhook.NewSimpleBuffer(parent.Type())
	buf.SetMeta(m.cfg.Freq)

	go func() {
		var count int = 0
		err := skyhook.ReadMultiple(ctx.Slice.Length(), m.cfg.Freq, parents, func(index int, datas []skyhook.Data) error {
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
			panic(fmt.Errorf("expected downsample length %d but got %d", expectedLength, count))
		}
		buf.Close()
	}()

	return buf
}

func (m Downsample) Close() {}

func init() {
	skyhook.Executors["downsample"] = NewDownsample
}
