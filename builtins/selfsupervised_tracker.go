package builtins

import (
	"../vaas"
)

type SelfSupervisedTrackerConfig struct {
	ModelPath string
}

func NewSelfSupervisedTracker(node vaas.Node) vaas.Executor {
	var cfg SelfSupervisedTrackerConfig
	vaas.JsonUnmarshal([]byte(node.Code), &cfg)

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
