package app

import (
	"testing"
)

func TestEnumerateConcreteRefs(t *testing.T) {
	test := func(inputs [][]Slice, expected []Slice) {
		refs := make([][]ConcreteRef, len(inputs))
		for i := range inputs {
			refs[i] = make([]ConcreteRef, len(inputs[i]))
			for j := range inputs[i] {
				refs[i][j] = ConcreteRef{Slice: inputs[i][j]}
			}
		}
		var outputs []Slice
		f := func(slice Slice, refs []ConcreteRef) {
			outputs = append(outputs, slice)
		}
		EnumerateConcreteRefs(refs, f)
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

	seg0 := Segment{ID: 0}
	seg1 := Segment{ID: 1}

	// simple
	test([][]Slice{
		{Slice{seg0, 0, 100}},
		{Slice{seg0, 50, 90}},
	}, []Slice{
		Slice{seg0, 50, 90},
	})

	// two segments, two slices each
	test([][]Slice{
		{
			Slice{seg0, 0, 100},
			Slice{seg1, 0, 100},
		},
		{
			Slice{seg0, 20, 40},
			Slice{seg0, 90, 100},
			Slice{seg1, 10, 20},
			Slice{seg1, 80, 90},
		},
	}, []Slice{
		Slice{seg0, 20, 40},
		Slice{seg0, 90, 100},
		Slice{seg1, 10, 20},
		Slice{seg1, 80, 90},
	})

	// complicated slices
	test([][]Slice{
		{
			Slice{seg0, 0, 50},
			Slice{seg0, 51, 100},
			Slice{seg0, 120, 130},
		},
		{
			Slice{seg0, 30, 60},
			Slice{seg0, 80, 90},
			Slice{seg0, 95, 100},
			Slice{seg0, 110, 140},
		},
	}, []Slice{
		Slice{seg0, 30, 50},
		Slice{seg0, 51, 60},
		Slice{seg0, 80, 90},
		Slice{seg0, 95, 100},
		Slice{seg0, 120, 130},
	})
}
