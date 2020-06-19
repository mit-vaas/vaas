package builtins

import (
	"../skyhook"
)

type CropConfig struct {
	Left int `json:"left"`
	Top int `json:"top"`
	Right int `json:"right"`
	Bottom int `json:"bottom"`
}

type Crop struct {
	cfg CropConfig
}

func NewCrop(node *skyhook.Node, query *skyhook.Query) skyhook.Executor {
	var cfg CropConfig
	skyhook.JsonUnmarshal([]byte(node.Code), &cfg)
	return Crop{cfg: cfg}
}

func (m Crop) Run(parents []skyhook.DataReader, slice skyhook.Slice) skyhook.DataBuffer {
	buf := skyhook.NewVideoBuffer()

	go func() {
		PerFrame(parents, slice, buf, skyhook.VideoType, func(idx int, data skyhook.Data, buf skyhook.DataBuffer) error {
			im := data.(skyhook.VideoData)[0]
			width := m.cfg.Right - m.cfg.Left
			height := m.cfg.Bottom - m.cfg.Top
			if width % 2 == 1 {
				width++
			}
			if height % 2 == 1 {
				height++
			}
			outim := skyhook.NewImage(width, height)
			for i := m.cfg.Left; i < m.cfg.Right; i++ {
				for j := m.cfg.Top; j < m.cfg.Bottom; j++ {
					outim.Set(i - m.cfg.Left, j - m.cfg.Top, im.Get(i, j))
				}
			}
			buf.Write(skyhook.VideoData{outim})
			return nil
		})
	}()

	return buf
}

func (m Crop) Close() {}

func init() {
	skyhook.Executors["crop"] = NewCrop
}
