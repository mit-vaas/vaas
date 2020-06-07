package skyhook

type ModelFunc func(cfgBytes []byte, parents [][]*LabelBuffer, slices []ClipSlice) []*LabelBuffer

var Models = make(map[string]ModelFunc)

type ModelConfig struct {
	ID string
}

func (op *Op) testModel(parents [][]*LabelBuffer, slices []ClipSlice) []*LabelBuffer {
	cfgBytes := []byte(op.Code)
	var cfg ModelConfig
	JsonUnmarshal(cfgBytes, &cfg)
	return Models[cfg.ID](cfgBytes, parents, slices)
}
