package main

import (
	"github.com/mitroadmaps/gomapinfer/common"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

type Detection struct {
	Left int `json:"left"`
	Top int `json:"top"`
	Right int `json:"right"`
	Bottom int `json:"bottom"`
}

func (d Detection) Rect() common.Rectangle {
	return common.Rectangle{
		common.Point{float64(d.Left), float64(d.Top)},
		common.Point{float64(d.Right), float64(d.Bottom)},
	}
}

const MaxMatchAge int = 10
var MatchLengths = []int{4, 8, 16}
var MaxMatchLength = MatchLengths[len(MatchLengths) - 1]

// Returns map idx (detection in frameIdx) -> (frame, det_idx)
func matchFrom(detections [][]Detection, frameIdx int) map[int]map[[2]int]bool {
	// from detection_idx in frameIdx to list of matching tuples (frame, det_idx)
	curMatches := make(map[int]map[[2]int]bool)
	finalMatches := make(map[int]map[[2]int]bool)
	for idx := range detections[frameIdx] {
		curMatches[idx] = make(map[[2]int]bool)
		finalMatches[idx] = make(map[[2]int]bool)
		curMatches[idx][[2]int{frameIdx, idx}] = true
		finalMatches[idx][[2]int{frameIdx, idx}] = true
	}

	lastFrame := frameIdx + MaxMatchLength
	for rightFrame := frameIdx + 1; rightFrame <= lastFrame; rightFrame++ {
		// find the detections we need to match
		checkSet := make(map[[2]int]bool)
		for _, matches := range curMatches {
			for t := range matches {
				if rightFrame - t[0] > MaxMatchAge {
					continue
				}
				checkSet[t] = true
			}
		}

		connections := make(map[[2]int][][2]int)
		for left := range checkSet {
			leftFrame, leftIdx := left[0], left[1]
			for rightIdx := 0; rightIdx < len(detections[rightFrame]); rightIdx++ {
				leftRect := detections[leftFrame][leftIdx].Rect()
				rightRect := detections[rightFrame][rightIdx].Rect()
				iouScore := leftRect.IOU(rightRect)
				if iouScore < 0.1 {
					continue
				}
				connections[left] = append(connections[left], [2]int{rightFrame, rightIdx})
			}
		}

		for idx, matches := range curMatches {
			for t := range matches {
				if rightFrame - t[0] >= MaxMatchAge {
					delete(matches, t)
				}
			}
			for left := range matches {
				for _, right := range connections[left] {
					matches[right] = true
					finalMatches[idx][right] = true
				}
			}
		}
	}

	return finalMatches
}

func process(jsonFname string, matchFname string) {
	bytes, err := ioutil.ReadFile(jsonFname)
	if err != nil {
		panic(err)
	}
	var detections [][]Detection
	if err := json.Unmarshal(bytes, &detections); err != nil {
		panic(err)
	}

	// match length -> frameIdx -> det_idx in frameIdx -> list of det_idx in (frameIdx+match length)
	matches := make(map[int]map[int]map[int][]int)
	mlSet := make(map[int]bool)
	for _, matchLength := range MatchLengths {
		mlSet[matchLength] = true
		matches[matchLength] = make(map[int]map[int][]int)
	}

	n := 8
	ch := make(chan int)
	donech := make(chan map[int]map[int]map[int][]int)
	for i := 0; i < n; i++ {
		go func() {
			threadMatches := make(map[int]map[int]map[int][]int)
			for _, matchLength := range MatchLengths {
				threadMatches[matchLength] = make(map[int]map[int][]int)
			}
			for baseFrame := range ch {
				ok := true
				for frameIdx := baseFrame; frameIdx <= baseFrame + MaxMatchLength; frameIdx++ {
					if len(detections[frameIdx]) == 0 {
						ok = false
					}
				}
				if !ok {
					continue
				}
				frameMatches := matchFrom(detections, baseFrame)
				for curIdx := range frameMatches {
					for right := range frameMatches[curIdx] {
						matchLength := right[0] - baseFrame
						rightIdx := right[1]
						if !mlSet[matchLength] {
							continue
						}
						if threadMatches[matchLength][baseFrame] == nil {
							threadMatches[matchLength][baseFrame] = make(map[int][]int)
						}
						threadMatches[matchLength][baseFrame][curIdx] = append(threadMatches[matchLength][baseFrame][curIdx], rightIdx)
					}
				}
			}
			donech <- threadMatches
		}()
	}
	for baseFrame := 0; baseFrame < len(detections) - MaxMatchLength; baseFrame++ {
		ch <- baseFrame
	}
	close(ch)
	for i := 0; i < n; i++ {
		threadMatches := <- donech
		for matchLength := range threadMatches {
			for baseFrame := range threadMatches[matchLength] {
				for curIdx := range threadMatches[matchLength][baseFrame] {
					if matches[matchLength][baseFrame] == nil {
						matches[matchLength][baseFrame] = make(map[int][]int)
					}
					matches[matchLength][baseFrame][curIdx] = threadMatches[matchLength][baseFrame][curIdx]
				}
			}
		}
	}

	bytes, err = json.Marshal(matches)
	if err != nil {
		panic(err)
	}
	if err := ioutil.WriteFile(matchFname, bytes, 0644); err != nil {
		panic(err)
	}
}

func main() {
	exportPath := os.Args[1]
	files, err := ioutil.ReadDir(exportPath)
	if err != nil {
		panic(err)
	}
	for _, fi := range files {
		if !strings.HasSuffix(fi.Name(), "_1.json") {
			continue
		}
		label := strings.Split(fi.Name(), "_1.json")[0]
		jsonFname := exportPath + label + "_1.json"
		matchFname := exportPath + label + "_match.json"
		process(jsonFname, matchFname)
	}
}
