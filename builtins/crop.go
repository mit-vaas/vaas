package builtins

import (
	"../skyhook"
	"fmt"
)

type CropConfig struct {
	Left int `json:"left"`
	Top int `json:"top"`
	Right int `json:"right"`
	Bottom int `json:"bottom"`
}

type Crop struct {
	node *skyhook.Node
	cfg CropConfig
}

func NewCrop(node *skyhook.Node) skyhook.Executor {
	var cfg CropConfig
	skyhook.JsonUnmarshal([]byte(node.Code), &cfg)
	return Crop{
		node: node,
		cfg: cfg,
	}
}

func (m Crop) Run(ctx skyhook.ExecContext) skyhook.DataBuffer {
	parents, err := GetParents(ctx, m.node)
	if err != nil {
		return skyhook.GetErrorBuffer(m.node.DataType, fmt.Errorf("crop error reading parents: %v", err))
	}

	w := skyhook.NewVideoWriter()

	go func() {
		w.SetMeta(parents[0].Freq())
		PerFrame(parents, ctx.Slice, w, skyhook.VideoType, func(idx int, data skyhook.Data, w skyhook.DataWriter) error {
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
					outim.SetRGB(i - m.cfg.Left, j - m.cfg.Top, im.GetRGB(i, j))
				}
			}
			w.Write(skyhook.VideoData{outim})
			return nil
		})
	}()

	return w.Buffer()
}

func (m Crop) Close() {}

func init() {
	skyhook.Executors["crop"] = NewCrop
}
