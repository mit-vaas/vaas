package skyhook

type ModelFunc func(cfgBytes []byte) Executor
var Models = make(map[string]ModelFunc)

type ModelConfig struct {
	ID string
}

func (node *Node) modelExecutor(query *Query) Executor {
	cfgBytes := []byte(node.Code)
	var cfg ModelConfig
	JsonUnmarshal(cfgBytes, &cfg)
	return Models[cfg.ID](cfgBytes)
}
