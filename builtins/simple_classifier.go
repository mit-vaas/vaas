package builtins

import (
	"../vaas"

	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"sync"
)

type SimpleClassifierConfig struct {
	ModelPath string
}

type SimpleClassifier struct {
	node vaas.Node
	cfg SimpleClassifierConfig
	stdin io.WriteCloser
	rd *bufio.Reader
	cmd *vaas.Cmd
	mu sync.Mutex
}

func NewSimpleClassifier(node vaas.Node) vaas.Executor {
	var cfg SimpleClassifierConfig
	vaas.JsonUnmarshal([]byte(node.Code), &cfg)
	return &SimpleClassifier{
		node: node,
		cfg: cfg,
	}
}

func (m *SimpleClassifier) start() {
	m.cmd = vaas.Command(
		"simple-classifier-run", vaas.CommandOptions{},
		"python3", "models/simple-classifier/run.py", m.cfg.ModelPath, "2",
	)
	m.stdin = m.cmd.Stdin()
	m.rd = bufio.NewReader(m.cmd.Stdout())
}

func (m *SimpleClassifier) Run(ctx vaas.ExecContext) vaas.DataBuffer {
	parents, err := GetParents(ctx, m.node)
	if err != nil {
		return vaas.GetErrorBuffer(m.node.DataType, fmt.Errorf("simple-classifier error reading parents: %v", err))
	}
	buf := vaas.NewSimpleBuffer(vaas.ClassType)

	go func() {
		buf.SetMeta(parents[0].Freq())
		PerFrame(parents, ctx.Slice, buf, vaas.VideoType, func(idx int, data vaas.Data, buf vaas.DataWriter) error {
			im := data.(vaas.VideoData)[0]
			m.mu.Lock()
			if m.stdin == nil {
				m.start()
			}
			header := make([]byte, 8)
			binary.BigEndian.PutUint32(header[0:4], uint32(im.Width))
			binary.BigEndian.PutUint32(header[4:8], uint32(im.Height))
			m.stdin.Write(header)
			m.stdin.Write(im.Bytes)
			line, err := m.rd.ReadString('\n')
			m.mu.Unlock()
			if err != nil {
				return fmt.Errorf("error reading from simple-classifier model: %v", err)
			}
			line = strings.TrimSpace(line)
			var pred []float64
			vaas.JsonUnmarshal([]byte(line), &pred)
			bestClass := 0
			var bestP float64 = 0.0
			for i, p := range pred {
				if p > bestP {
					bestP = p
					bestClass = i
				}
			}
			buf.Write(vaas.ClassData{bestClass})
			return nil
		})
	}()

	return buf
}

func (m *SimpleClassifier) Close() {
	m.stdin.Close()
	m.cmd.Wait()
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
	}
}
