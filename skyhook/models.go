package skyhook

type ModelFunc func(cfgBytes []byte, parents [][]*LabelBuffer, slices []ClipSlice) []*LabelBuffer

var Models = make(map[string]ModelFunc)

type ModelConfig struct {
	ID string
}

func (node *Node) testModel(query *Query, parents [][]*LabelBuffer, slices []ClipSlice) []*LabelBuffer {
	cfgBytes := []byte(node.Code)
	var cfg ModelConfig
	JsonUnmarshal(cfgBytes, &cfg)
	return Models[cfg.ID](cfgBytes, parents, slices)
}
