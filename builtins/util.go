package builtins

import (
	"../vaas"

	"fmt"
	"log"
)

func GetParents(ctx vaas.ExecContext, node vaas.Node) ([]vaas.DataReader, error) {
	parents := make([]vaas.DataReader, len(node.Parents))
	for i, parent := range node.Parents {
		if parent.Type == vaas.NodeParent {
			rd, err := ctx.GetReader(*ctx.Nodes[parent.NodeID])
			if err != nil {
				return nil, err
			}
			parents[i] = rd
		} else if parent.Type == vaas.SeriesParent {
			buf := &vaas.VideoFileBuffer{ctx.Inputs[parent.SeriesIdx], ctx.Slice}
			parents[i] = buf.Reader()
		}
	}
	return parents, nil
}

type PerFrameFunc func(idx int, data vaas.Data, outBuf vaas.DataWriter) error

func PerFrame(parents []vaas.DataReader, slice vaas.Slice, buf vaas.DataWriter, t vaas.DataType, f PerFrameFunc) {
	err := vaas.ReadMultiple(slice.Length(), vaas.MinFreq(parents), parents, func(index int, datas []vaas.Data) error {
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
