package skyhook

import (
	gomapinfer "github.com/mitroadmaps/gomapinfer/common"
)

func DetectionToRectangle(d Detection) gomapinfer.Rectangle {
	return gomapinfer.Rectangle{
		gomapinfer.Point{float64(d.Left), float64(d.Top)},
		gomapinfer.Point{float64(d.Right), float64(d.Bottom)},
	}
}

// first argument is ground truth, second is outputs
type MetricFunc func([]Data, []Data) float64
var Metrics = make(map[string]MetricFunc)

func AvgMetric(f func(Data, Data) float64) MetricFunc {
	return func(a []Data, b []Data) float64 {
		var sum float64 = 0
		for i := range a {
			sum += f(a[i], b[i])
		}
		return sum / float64(len(a))
	}
}

func AvgMetricPerFrame(f func(Data, Data) float64) MetricFunc {
	return func(alist []Data, blist []Data) float64 {
		var samples []float64
		for i := range alist {
			for j := 0; j < alist[i].Length(); j++ {
				acur := alist[i].Slice(j, j+1)
				bcur := blist[i].Slice(j, j+1)
				samples = append(samples, f(acur, bcur))
			}
		}
		var sum float64 = 0
		for _, x := range samples {
			sum += x
		}
		return sum / float64(len(samples))
	}
}

func DetectionF1(iouThreshold float64) MetricFunc {
	return AvgMetricPerFrame(func(a Data, b Data) float64 {
		alist := a.(DetectionData)[0]
		blist := b.(DetectionData)[0]
		bset := make(map[int]bool)
		matches := 0
		for _, adet := range alist {
			var bestIdx int = -1
			var bestIOU float64
			for bidx, bdet := range blist {
				if bset[bidx] {
					continue
				}
				iou := DetectionToRectangle(adet).IOU(DetectionToRectangle(bdet))
				if iou < iouThreshold {
					continue
				}
				if bestIdx == -1 || iou > bestIOU {
					bestIdx = bidx
					bestIOU = iou
				}
			}
			if bestIdx != -1 {
				matches++
				bset[bestIdx] = true
			}
		}
		if matches == 0 {
			return 0
		}
		precision := float64(matches) / float64(len(blist))
		recall := float64(matches) / float64(len(alist))
		return 2 * precision * recall / (precision + recall)
	})
}

func init() {
	Metrics["detectionf1-50"] = DetectionF1(0.5)
}
