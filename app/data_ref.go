package app

import (
	"../vaas"

	"fmt"
	"sort"
)

type DataRef struct {
	Series *vaas.Series // reference an entire series
	Item *vaas.Item // reference a specific item

	// include the data directly
	Slice vaas.Slice
	DataType vaas.DataType
	Data string
}

type ConcreteRef struct {
	Slice vaas.Slice

	Item *vaas.Item
	Data vaas.Data
}

func (ref ConcreteRef) Type() vaas.DataType {
	if ref.Item != nil {
		return ref.Item.Series.DataType
	} else {
		return ref.Data.Type()
	}
}

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

type ConcreteRefs []ConcreteRef
func (a ConcreteRefs) Len() int           { return len(a) }
func (a ConcreteRefs) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ConcreteRefs) Less(i, j int) bool {
	if a[i].Slice.Segment.ID < a[j].Slice.Segment.ID {
		return true
	} else if a[i].Slice.Segment.ID > a[j].Slice.Segment.ID {
		return false
	} else if a[i].Slice.Start < a[j].Slice.Start {
		return true
	}
	return false
}

// Essentially, find the intersection of a bunch of sets of intervals (slices).
func EnumerateConcreteRefs(refs [][]ConcreteRef, f func(vaas.Slice, []ConcreteRef) error) error {
	// collect segments where all the sets have at least one ConcreteRef
 	vsize := len(refs)
	bySegment := make(map[int][][]ConcreteRef)
	for i := 0; i < vsize; i++ {
		for _, ref := range refs[i] {
			segmentID := ref.Slice.Segment.ID
			if len(bySegment[segmentID]) < i {
				delete(bySegment, segmentID)
				continue
			}
			if len(bySegment[segmentID]) == i {
				bySegment[segmentID] = append(bySegment[segmentID], []ConcreteRef{})
			}
			bySegment[segmentID][i] = append(bySegment[segmentID][i], ref)
		}
	}

	// enumerate in each segment
	for _, cur := range bySegment {
		for i := 0; i < vsize; i++ {
			sort.Sort(ConcreteRefs(cur[i]))
		}

		indexes := make([]int, vsize)
		t := 0

		iter := func() (bool, error) {
			// set t to max left index
			for i := 0; i < vsize; i++ {
				if cur[i][indexes[i]].Slice.Start > t {
					t = cur[i][indexes[i]].Slice.Start
				}
			}

			// increment until right index is right of t
			for i := 0; i < vsize; i++ {
				for indexes[i] < len(cur[i]) && cur[i][indexes[i]].Slice.End <= t {
					indexes[i]++
				}
				if indexes[i] >= len(cur[i]) {
					return true, nil
				}
			}

			// see if we got an interval
			var l []ConcreteRef
			end := -1
			constrainingIdx := -1
			for i := 0; i < vsize; i++ {
				if cur[i][indexes[i]].Slice.Start > t || cur[i][indexes[i]].Slice.End <= t {
					break
				}
				l = append(l, cur[i][indexes[i]])
				if end == -1 || cur[i][indexes[i]].Slice.End < end {
					end = cur[i][indexes[i]].Slice.End
					constrainingIdx = i
				}
			}
			if len(l) < vsize {
				return false, nil
			}
			slice := vaas.Slice{l[0].Slice.Segment, t, end}
			err := f(slice, l)
			if err != nil {
				return true, err
			}

			// increment the one with smallest right index
			indexes[constrainingIdx]++
			if indexes[constrainingIdx] >= len(cur[constrainingIdx]) {
				return true, nil
			}
			return false, nil
		}

		for {
			stop, err := iter()
			if err != nil {
				return err
			} else if stop {
				break
			}
		}
	}

	return nil
}
