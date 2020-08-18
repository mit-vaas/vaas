package builtins

import (
	"../vaas"

	"encoding/json"
	"fmt"
	"strconv"
	"sync"
)

type SimpleClassifierConfig struct {
	ModelPath string
	NumClasses int
	InputSize [2]int
}

type SimpleClassifier struct {
	node vaas.Node
	cfg SimpleClassifierConfig
	mu sync.Mutex
	stats *vaas.StatsHolder
}

func NewSimpleClassifier(node vaas.Node) vaas.Executor {
	var cfgs []SimpleClassifierConfig
	err := json.Unmarshal([]byte(node.Code), &cfgs)
	if err != nil {
		return vaas.ErrorExecutor{node.DataType, fmt.Errorf("error decoding node configuration: %v", err)}
	}
	cfg := cfgs[0]

	cmd := vaas.Command(
		"simple-classifier-run", vaas.CommandOptions{},
		"python3", "models/simple-classifier/run.py",
		cfg.ModelPath, strconv.Itoa(cfg.NumClasses), strconv.Itoa(cfg.InputSize[0]), strconv.Itoa(cfg.InputSize[1]),
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
	vaas.Executors["simple-classifier"] = vaas.ExecutorMeta{
		New: NewSimpleClassifier,
		Environment: &vaas.Environment{
			Template: "gpu",
			Requirements: map[string]int{
				"container": 1,
				"gpu": 1,
			},
		},
		Tune: func(node vaas.Node, gtlist []vaas.Data) [][2]string {
			var cfgs []SimpleClassifierConfig
			vaas.JsonUnmarshal([]byte(node.Code), &cfgs)

			var descs [][2]string
			for _, cfg := range cfgs {
				encoded := vaas.JsonMarshal([]SimpleClassifierConfig{cfg})
				descs = append(descs, [2]string{
					string(encoded),
					fmt.Sprintf("%dx%d", cfg.InputSize[0], cfg.InputSize[1]),
				})
			}
			return descs
		},
	}
}
