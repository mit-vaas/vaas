package builtins

import (
	"../vaas"

	"fmt"
	"log"
)

func GetParent(ctx vaas.ExecContext, parent vaas.Parent) (vaas.DataReader, error) {
	if parent.Type == vaas.NodeParent {
		rd, err := ctx.GetReader(*ctx.Nodes[parent.NodeID])
		if err != nil {
			return nil, err
		}
		return rd, nil
	} else if parent.Type == vaas.SeriesParent {
		item := ctx.Inputs[parent.SeriesIdx]
		if item.Format == "json" {
			return item.Load(ctx.Slice).Reader(), nil
		} else {
			buf := &vaas.VideoFileBuffer{item, ctx.Slice}
			return buf.Reader(), nil
		}
	}
	return nil, fmt.Errorf("invalid parent type %v", parent.Type)
}

func GetParents(ctx vaas.ExecContext, node vaas.Node) ([]vaas.DataReader, error) {
	parents := make([]vaas.DataReader, len(node.Parents))
	for i, parent := range node.Parents {
		var err error
		parents[i], err = GetParent(ctx, parent)
		if err != nil {
			return nil, err
		}
	}
	return parents, nil
}

type PerFrameFunc func(idx int, data vaas.Data, outBuf vaas.DataWriter) error

func PerFrame(parents []vaas.DataReader, slice vaas.Slice, buf vaas.DataWriter, t vaas.DataType, opts vaas.ReadMultipleOptions, f PerFrameFunc) {
	err := vaas.ReadMultiple(slice.Length(), vaas.MinFreq(parents), parents, opts, func(index int, datas []vaas.Data) error {
		if len(datas) != 1 {
			panic(fmt.Errorf("expected exactly one input, but got %d", len(datas)))
		} else if datas[0].Type() != t {
			panic(fmt.Errorf("expected type %v", t))
		}
		for i := 0; i < datas[0].Length(); i++ {
			err := f(index+i, datas[0].Slice(i, i+1), buf)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("[models (%v)] error reading: %v", slice, err)
		buf.Error(err)
	}
	buf.Close()
}
