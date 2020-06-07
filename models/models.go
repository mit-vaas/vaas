package models

import (
	"../skyhook"

	"fmt"
	"log"
)

type PerFrameFunc func(im skyhook.Image, outBuf *skyhook.LabelBuffer) error

func PerFrame(parents [][]*skyhook.LabelBuffer, slices []skyhook.ClipSlice, buffers []*skyhook.LabelBuffer, f PerFrameFunc) {
	for i, slice := range slices {
		completed := 0
		stop := false
		for !stop && completed < slice.Length() {
			data, err := parents[0][i].Read(completed, 0)
			if err != nil {
				log.Printf("[models (%v)] error reading from parent: %v", slice, err)
				buffers[i].Error(err)
				break
			}

			if data.Type != skyhook.VideoType {
				panic(fmt.Errorf("expected video type"))
			}

			completed += data.Length()
			for _, im := range data.Images {
				err := f(im, buffers[i])
				if err != nil {
					log.Printf("[models (%v)] func error: %v", slice, err)
					buffers[i].Error(err)
					stop = true
					break
				}
			}
		}
	}
}
