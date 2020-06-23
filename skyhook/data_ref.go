package skyhook

type DataRef struct {
	Series *Series // reference an entire series
	Item *Item // reference a specific item

	// include the data directly
	Slice Slice
	DataType DataType
	Data string
}

type ConcreteRef struct {
	Slice Slice

	Item *Item
	Data Data
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
func EnumerateConcreteRefs(refs [][]ConcreteRef, f func(Slice, []ConcreteRef)) {
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
		indexes := make([]int, vsize)
		t := 0

		iter := func() bool {
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
					return true
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
				return false
			}
			slice := Slice{l[0].Slice.Segment, t, end}
			f(slice, l)

			// increment the one with smallest right index
			indexes[constrainingIdx]++
			if indexes[constrainingIdx] >= len(cur[constrainingIdx]) {
				return true
			}
			return false
		}

		for {
			stop := iter()
			if stop {
				break
			}
		}
	}
}
