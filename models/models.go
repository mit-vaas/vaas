package models

import (
	"../skyhook"

	"fmt"
	"log"
)

type PerFrameFunc func(idx int, data skyhook.Data, outBuf *skyhook.DataBuffer) error

func PerFrame(parents []*skyhook.BufferReader, slice skyhook.Slice, buf *skyhook.DataBuffer, t skyhook.DataType, f PerFrameFunc) {
	err := skyhook.ReadMultiple(slice.Length(), parents, func(index int, datas []skyhook.Data) error {
		if len(datas) != 1 {
			panic(fmt.Errorf("expected exactly one input, but got %d", len(datas)))
		} else if datas[0].Type != t {
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
}
