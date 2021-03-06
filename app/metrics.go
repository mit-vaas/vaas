package app

import (
	"../vaas"
	"math"
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
	return AvgMetricPerFrame(func(a_ vaas.Data, b_ vaas.Data) float64 {
		a := a_.(vaas.DetectionData)
		b := b_.(vaas.DetectionData).Resize(a.D[0].CanvasDims)
		alist := a.D[0].Detections
		blist := b.D[0].Detections
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
		acls := a.(vaas.IntData)[0]
		bcls := b.(vaas.IntData)[0]
		if acls == bcls {
			return 1.0
		} else {
			return 0.0
		}
	})
}

func MinOverMax() MetricFunc {
	return AvgMetricPerFrame(func(data1 vaas.Data, data2 vaas.Data) float64 {
		x1 := data1.(vaas.IntData)[0]
		x2 := data2.(vaas.IntData)[0]
		if x1 == x2 {
			return 1.0
		} else {
			return math.Min(float64(x1), float64(x2)) / math.Max(float64(x1), float64(x2))
		}
	})
}

func init() {
	Metrics["detectionf1-50"] = DetectionF1(0.5)
	Metrics["class-accuracy"] = ClassAccuracy()
	Metrics["min-over-max"] = MinOverMax()
}
