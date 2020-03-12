package main

import (
	"github.com/mitroadmaps/gomapinfer/image"

	"log"
)

var Colors = [][3]uint8{
	[3]uint8{255, 0, 0},
	[3]uint8{0, 255, 0},
	[3]uint8{0, 0, 255},
	[3]uint8{255, 255, 0},
	[3]uint8{0, 255, 255},
	[3]uint8{255, 0, 255},
	[3]uint8{0, 51, 51},
	[3]uint8{51, 153, 153},
	[3]uint8{102, 0, 51},
	[3]uint8{102, 51, 204},
	[3]uint8{102, 153, 204},
	[3]uint8{102, 255, 204},
	[3]uint8{153, 102, 102},
	[3]uint8{204, 102, 51},
	[3]uint8{204, 255, 102},
	[3]uint8{255, 255, 204},
	[3]uint8{121, 125, 127},
	[3]uint8{69, 179, 157},
	[3]uint8{250, 215, 160},
}

type PreviewClip struct {
	Images []Image
	Type DataType
	Label interface{}
}

func (clipLabel *LabeledClip) LoadPreview() *PreviewClip {
	slice := clipLabel.Slice
	images := GetFrames(slice.Clip, slice.Start, slice.End, slice.Clip.Width, slice.Clip.Height)
	if clipLabel.Type == DetectionType || clipLabel.Type == TrackType {
		detections := clipLabel.Label.([][]Detection)
		for i := range detections {
			for _, detection := range detections[i] {
				var color [3]uint8
				if clipLabel.Type == DetectionType {
					color = [3]uint8{255, 0, 0}
				} else {
					color = Colors[mod(detection.TrackID, len(Colors))]
				}
				image.DrawRectangle(images[i].Image, detection.Left, detection.Top, detection.Right, detection.Bottom, 2, color)
			}
		}
	}
	return &PreviewClip{
		Images: images,
		Type: clipLabel.Type,
		Label: clipLabel.Label,
	}
}

func (pc *PreviewClip) GetVideo() []byte {
	log.Printf("[preview] convert %d images to video", len(pc.Images))
	return MakeVideo(pc.Images)
}
