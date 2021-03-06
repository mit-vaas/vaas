package builtins

import (
	"../vaas"

	"encoding/json"
	"fmt"
)

type SelfSupervisedTrackerConfig struct {
	ModelPath string
}

func NewSelfSupervisedTracker(node vaas.Node) vaas.Executor {
	var cfg SelfSupervisedTrackerConfig
	err := json.Unmarshal([]byte(node.Code), &cfg)
	if err != nil {
		return vaas.ErrorExecutor{node.DataType, fmt.Errorf("error decoding node configuration: %v", err)}
	}

	cmd := vaas.Command(
		"selfsupervised-tracker-run", vaas.CommandOptions{},
		"python3", "models/selfsupervised-tracker/run.py",
		cfg.ModelPath,
	)

	e := &PythonExecutor{
		node: node,
		cmd: cmd,
		stdin: cmd.Stdin(),
		stdout: cmd.Stdout(),
		pending: make(map[int]*pendingSlice),
		stats: new(vaas.StatsHolder),
	}
	e.Init()
	go e.ReadLoop()
	return e
}

func init() {
	vaas.Executors["selfsupervised-tracker"] = vaas.ExecutorMeta{
		New: NewSelfSupervisedTracker,
		Environment: &vaas.Environment{
			Template: "gpu",
			Requirements: map[string]int{
				"container": 1,
				"gpu": 1,
			},
		},
	}
}
