package app

import (
	"../vaas"
)

// first argument is ground truth, second is outputs
type MetricFunc func([]vaas.Data, []vaas.Data) float64
var Metrics = make(map[string]MetricFunc)

func AvgMetric(f func(vaas.Data, vaas.Data) float64) MetricFunc {
	return func(a []vaas.Data, b []vaas.Data) float64 {
		var sum float64 = 0
		for i := range a {
			sum += f(a[i], b[i])
		}
		return sum / float64(len(a))
	}
}

func AvgMetricPerFrame(f func(vaas.Data, vaas.Data) float64) MetricFunc {
	return func(alist []vaas.Data, blist []vaas.Data) float64 {
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
	return AvgMetricPerFrame(func(a vaas.Data, b vaas.Data) float64 {
		alist := a.(vaas.DetectionData)[0]
		blist := b.(vaas.DetectionData)[0]
		bset := make(map[int]bool)
		matches := 0
		for _, adet := range alist {
			var bestIdx int = -1
			var bestIOU float64
			for bidx, bdet := range blist {
				if bset[bidx] {
					continue
				}
				iou := vaas.DetectionToRectangle(adet).IOU(vaas.DetectionToRectangle(bdet))
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

func ClassAccuracy() MetricFunc {
	return AvgMetricPerFrame(func(a vaas.Data, b vaas.Data) float64 {
		acls := a.(vaas.ClassData)[0]
		bcls := b.(vaas.ClassData)[0]
		if acls == bcls {
			return 1.0
		} else {
			return 0.0
		}
	})
}

func init() {
	Metrics["detectionf1-50"] = DetectionF1(0.5)
	Metrics["class-accuracy"] = ClassAccuracy()
}
