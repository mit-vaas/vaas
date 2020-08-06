package app

import (
	"../vaas"
	"testing"
)

func TestSliceIntersection(t *testing.T) {
	test := func(inputs [][]vaas.Slice, expected []vaas.Slice) {
		outputs := SliceIntersection(inputs)
		if len(outputs) != len(expected) {
			t.Fatalf("%v: output %v but expected %v", inputs, outputs, expected)
		}
		match := true
		for i := range outputs {
			if outputs[i] != expected[i] {
				match = false
			}
		}
		if !match {
			t.Fatalf("%v: output %v but expected %v", inputs, outputs, expected)
		}
	}

	seg0 := vaas.Segment{ID: 0}
	seg1 := vaas.Segment{ID: 1}

	// simple
	test([][]vaas.Slice{
		{vaas.Slice{seg0, 0, 100}},
		{vaas.Slice{seg0, 50, 90}},
	}, []vaas.Slice{
		vaas.Slice{seg0, 50, 90},
	})

	// two segments, two slices each
	test([][]vaas.Slice{
		{
			vaas.Slice{seg0, 0, 100},
			vaas.Slice{seg1, 0, 100},
		},
		{
			vaas.Slice{seg0, 20, 40},
			vaas.Slice{seg0, 90, 100},
			vaas.Slice{seg1, 10, 20},
			vaas.Slice{seg1, 80, 90},
		},
	}, []vaas.Slice{
		vaas.Slice{seg0, 20, 40},
		vaas.Slice{seg0, 90, 100},
		vaas.Slice{seg1, 10, 20},
		vaas.Slice{seg1, 80, 90},
	})

	// complicated slices
	test([][]vaas.Slice{
		{
			vaas.Slice{seg0, 0, 50},
			vaas.Slice{seg0, 51, 100},
			vaas.Slice{seg0, 120, 130},
		},
		{
			vaas.Slice{seg0, 30, 60},
			vaas.Slice{seg0, 80, 90},
			vaas.Slice{seg0, 95, 100},
			vaas.Slice{seg0, 110, 140},
		},
	}, []vaas.Slice{
		vaas.Slice{seg0, 30, 50},
		vaas.Slice{seg0, 51, 60},
		vaas.Slice{seg0, 80, 90},
		vaas.Slice{seg0, 95, 100},
		vaas.Slice{seg0, 120, 130},
	})
}
