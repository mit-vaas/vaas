package app

import (
	"../vaas"

	"sort"
)

type VaasSlices []vaas.Slice
func (a VaasSlices) Len() int           { return len(a) }
func (a VaasSlices) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a VaasSlices) Less(i, j int) bool {
	// sort by segment, then start, and finally prefer longer slices first
	if a[i].Segment.ID < a[j].Segment.ID {
		return true
	} else if a[i].Segment.ID > a[j].Segment.ID {
		return false
	} else if a[i].Start < a[j].Start {
		return true
	} else if a[i].Start > a[j].Start {
		return false
	} else if a[i].End > a[j].End {
		return true
	}
	return false
}

// Find the intersection of a bunch of sets of slices.
func SliceIntersection(sets [][]vaas.Slice) []vaas.Slice {
	// group the sets by segment
 	num := len(sets)
	bySegment := make(map[int][][]vaas.Slice)
	for setIdx := 0; setIdx < num; setIdx++ {
		for _, slice := range sets[setIdx] {
			segmentID := slice.Segment.ID
			if len(bySegment[segmentID]) < setIdx {
				continue
			}
			if len(bySegment[segmentID]) == setIdx {
				bySegment[segmentID] = append(bySegment[segmentID], []vaas.Slice{})
			}
			bySegment[segmentID][setIdx] = append(bySegment[segmentID][setIdx], slice)
		}
	}
	for segmentID := range bySegment {
		if len(bySegment[segmentID]) < num {
			delete(bySegment, segmentID)
			continue
		}
	}

	// enumerate in each segment
	var slices []vaas.Slice
	for _, cur := range bySegment {
		for i := 0; i < num; i++ {
			sort.Sort(VaasSlices(cur[i]))
		}

		indexes := make([]int, num)
		t := 0

		// Returns true if we exhausted the slices in this segment.
		// (i.e. should quit the outer loop)
		iter := func() bool {
			// set t to max left index
			for setIdx := 0; setIdx < num; setIdx++ {
				if cur[setIdx][indexes[setIdx]].Start > t {
					t = cur[setIdx][indexes[setIdx]].Start
				}
			}

			// increment until right index is right of t
			for setIdx := 0; setIdx < num; setIdx++ {
				for indexes[setIdx] < len(cur[setIdx]) && cur[setIdx][indexes[setIdx]].End <= t {
					indexes[setIdx]++
				}
				if indexes[setIdx] >= len(cur[setIdx]) {
					return true
				}
			}

			// see if we got an interval
			end := -1
			constrainingSet := -1
			for setIdx := 0; setIdx < num; setIdx++ {
				if cur[setIdx][indexes[setIdx]].Start > t || cur[setIdx][indexes[setIdx]].End <= t {
					end = -1
					break
				}
				if end == -1 || cur[setIdx][indexes[setIdx]].End < end {
					end = cur[setIdx][indexes[setIdx]].End
					constrainingSet = setIdx
				}
			}
			if end == -1 {
				return false
			}
			slice := vaas.Slice{cur[0][indexes[0]].Segment, t, end}
			slices = append(slices, slice)

			// increment the set index with smallest right index
			indexes[constrainingSet]++
			if indexes[constrainingSet] >= len(cur[constrainingSet]) {
				return true
			}
			return false
		}

		for !iter() {}
	}

	return slices
}
