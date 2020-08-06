package app

import (
	"../vaas"

	"fmt"
)

type DataRef struct {
	Series *vaas.Series // reference an entire series
	Item *vaas.Item // reference a specific item

	// include the data directly
	Slice vaas.Slice
	DataType vaas.DataType
	Data string
}

// References data for a specific slice.
// If item is available, Item is set.
// If data is in-memory, Data is set.
// Series is set if Series.RequireData needs to be called. (item may or may not exist)
type ConcreteRef struct {
	Slice vaas.Slice

	Item *vaas.Item
	Series *vaas.Series
	Data vaas.Data
}

func (ref ConcreteRef) Type() vaas.DataType {
	if ref.Item != nil {
		return ref.Item.Series.DataType
	} else if ref.Series != nil {
		return ref.Series.DataType
	} else {
		return ref.Data.Type()
	}
}

// Lists items and in-memory data provided in the DataRefs.
// All ConcreteRefs either have Item or Data set (not Series).
func ResolveDataRefs(refs []DataRef) ([]ConcreteRef, error) {
	var out []ConcreteRef

	for i := 0; i < len(refs); i++ {
		ref := refs[i]
		if ref.Series != nil {
			series := GetSeries(ref.Series.ID)
			if series == nil {
				return nil, fmt.Errorf("no such series %d", ref.Series.ID)
			}
			for _, item := range series.ListItems() {
				cur := item.Item
				refs = append(refs, DataRef{Item: &cur})
			}
		} else if ref.Item != nil {
			item := GetItem(ref.Item.ID)
			if item == nil {
				return nil, fmt.Errorf("no such item %d", ref.Item.ID)
			}
			out = append(out, ConcreteRef{
				Slice: item.Slice,
				Item: &item.Item,
			})
		} else if len(ref.Data) > 0 {
			segment := GetSegment(ref.Slice.Segment.ID)
			slice := vaas.Slice{segment.Segment, ref.Slice.Start, ref.Slice.End}
			data := vaas.DecodeData(ref.DataType, []byte(ref.Data)).EnsureLength(slice.Length())
			out = append(out, ConcreteRef{
				Slice: slice,
				Data: data,
			})
		} else {
			return nil, fmt.Errorf("bad DataRef")
		}
	}

	return out, nil
}

func EnumerateDataRefs(refs [][]DataRef, f func(vaas.Slice, []ConcreteRef) error) error {
	concretes := make([][]ConcreteRef, len(refs))
	for i := range refs {
		var err error
		concretes[i], err = ResolveDataRefs(refs[i])
		if err != nil {
			return err
		}
	}
	return EnumerateConcreteRefs(concretes, f)
}

// Group the ConcreteRefs by slices where they overlap.
func EnumerateConcreteRefs(refs [][]ConcreteRef, f func(vaas.Slice, []ConcreteRef) error) error {
	sets := make([][]vaas.Slice, len(refs))
	for i := range refs {
		sets[i] = make([]vaas.Slice, len(refs[i]))
		for j := range refs[i] {
			sets[i][j] = refs[i][j].Slice
		}
	}
	slices := SliceIntersection(sets)

	// collect the ConcreteRef for each slice
	for _, slice := range slices {
		var cur []ConcreteRef
		for i := 0; i < len(refs); i++ {
			for _, ref := range refs[i] {
				if !ref.Slice.Contains(slice) {
					continue
				}
				cur = append(cur, ref)
				break
			}
		}
		if len(cur) != len(sets) {
			panic(fmt.Errorf("EnumerateConcreteRefs could not find ref for some intersection slice"))
		}
		err := f(slice, cur)
		if err != nil {
			return err
		}
	}

	return nil
}
