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

type TunableClassifierConfig struct {
	ModelPath string
	MaxWidth int
	MaxHeight int
	NumClasses int
	ScaleCount int
	Depth int
}

type TunableClassifier struct {
	node vaas.Node
	cfg TunableClassifierConfig
	stdin io.WriteCloser
	rd *bufio.Reader
	cmd *vaas.Cmd
	mu sync.Mutex
}

func NewTunableClassifier(node vaas.Node) vaas.Executor {
	var cfg TunableClassifierConfig
	vaas.JsonUnmarshal([]byte(node.Code), &cfg)
	return &TunableClassifier{
		node: node,
		cfg: cfg,
	}
}

func (m *TunableClassifier) start() {
	m.cmd = vaas.Command(
		"tunable-classifier-run", vaas.CommandOptions{},
		"python3", "models/tunable-classifier/run.py",
		m.cfg.ModelPath,
		fmt.Sprintf("%d", m.cfg.NumClasses),
		fmt.Sprintf("%d", m.cfg.MaxWidth),
		fmt.Sprintf("%d", m.cfg.MaxHeight),
		fmt.Sprintf("%d", m.cfg.ScaleCount),
		fmt.Sprintf("%d", m.cfg.Depth),
	)
	m.stdin = m.cmd.Stdin()
	m.rd = bufio.NewReader(m.cmd.Stdout())
}

func (m *TunableClassifier) Run(ctx vaas.ExecContext) vaas.DataBuffer {
	parents, err := GetParents(ctx, m.node)
	if err != nil {
		return vaas.GetErrorBuffer(m.node.DataType, fmt.Errorf("tunable-classifier error reading parents: %v", err))
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
				return fmt.Errorf("error reading from tunable-classifier model: %v", err)
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

func (m *TunableClassifier) Close() {
	m.stdin.Close()
	m.cmd.Wait()
}

func init() {
	vaas.Executors["tunable-classifier"] = vaas.ExecutorMeta{
		New: NewTunableClassifier,
		Environment: &vaas.Environment{
			Template: "gpu",
			Requirements: map[string]int{
				"container": 1,
				"gpu": 1,
			},
		},
	}
}
