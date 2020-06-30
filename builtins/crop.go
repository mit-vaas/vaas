package builtins

import (
	"../vaas"
	"fmt"
)

type CropConfig struct {
	Left int `json:"left"`
	Top int `json:"top"`
	Right int `json:"right"`
	Bottom int `json:"bottom"`
}

type Crop struct {
	node vaas.Node
	cfg CropConfig
}

func NewCrop(node vaas.Node) vaas.Executor {
	var cfg CropConfig
	vaas.JsonUnmarshal([]byte(node.Code), &cfg)
	return Crop{
		node: node,
		cfg: cfg,
	}
}

func (m Crop) Run(ctx vaas.ExecContext) vaas.DataBuffer {
	parents, err := GetParents(ctx, m.node)
	if err != nil {
		return vaas.GetErrorBuffer(m.node.DataType, fmt.Errorf("crop error reading parents: %v", err))
	}

	w := vaas.NewVideoWriter()

	go func() {
		w.SetMeta(parents[0].Freq())
		PerFrame(parents, ctx.Slice, w, vaas.VideoType, func(idx int, data vaas.Data, w vaas.DataWriter) error {
			im := data.(vaas.VideoData)[0]
			width := m.cfg.Right - m.cfg.Left
			height := m.cfg.Bottom - m.cfg.Top
			if width % 2 == 1 {
				width++
			}
			if height % 2 == 1 {
				height++
			}
			outim := vaas.NewImage(width, height)
			for i := m.cfg.Left; i < m.cfg.Right; i++ {
				for j := m.cfg.Top; j < m.cfg.Bottom; j++ {
					outim.SetRGB(i - m.cfg.Left, j - m.cfg.Top, im.GetRGB(i, j))
				}
			}
			w.Write(vaas.VideoData{outim})
			return nil
		})
	}()

	return w.Buffer()
}

func (m Crop) Close() {}

func init() {
	vaas.Executors["crop"] = NewCrop
}
