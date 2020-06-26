package builtins

import (
	"../skyhook"

	"fmt"
	"log"
)

func GetParents(ctx skyhook.ExecContext, node *skyhook.Node) ([]skyhook.DataReader, error) {
	parents := make([]skyhook.DataReader, len(node.Parents))
	for i, parent := range node.Parents {
		if parent.Type == skyhook.NodeParent {
			rd, err := ctx.GetReader(ctx.Nodes[parent.NodeID])
			if err != nil {
				return nil, err
			}
			parents[i] = rd
		} else if parent.Type == skyhook.SeriesParent {
			buf := &skyhook.VideoFileBuffer{ctx.Inputs[parent.SeriesIdx], ctx.Slice}
			parents[i] = buf.Reader()
		}
	}
	return parents, nil
}

type PerFrameFunc func(idx int, data skyhook.Data, outBuf skyhook.DataWriter) error

func PerFrame(parents []skyhook.DataReader, slice skyhook.Slice, buf skyhook.DataWriter, t skyhook.DataType, f PerFrameFunc) {
	err := skyhook.ReadMultiple(slice.Length(), skyhook.MinFreq(parents), parents, func(index int, datas []skyhook.Data) error {
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
