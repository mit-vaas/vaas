package builtins

import (
	"../vaas"

	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

type TunableClassifierConfig struct {
	ModelPath string
	MaxWidth int
	MaxHeight int
	NumClasses int
	ScaleCount int
	Depth int
}

func (cfg TunableClassifierConfig) GetDims() [][2]int {
	cur := [2]int{cfg.MaxWidth, cfg.MaxHeight}
	var dims [][2]int
	for cur[0] >= 4 && cur[1] >= 4 {
		dims = append(dims, cur)
		cur[0] = (cur[0]-2)/2
		cur[1] = (cur[1]-2)/2
	}
	return dims
}

type TunableClassifier struct {
	node vaas.Node
	cfg TunableClassifierConfig
	stdin io.WriteCloser
	rd *bufio.Reader
	cmd *vaas.Cmd
	mu sync.Mutex
	stats *vaas.StatsHolder
}

func NewTunableClassifier(node vaas.Node) vaas.Executor {
	var cfg TunableClassifierConfig
	err := json.Unmarshal([]byte(node.Code), &cfg)
	if err != nil {
		return vaas.ErrorExecutor{node.DataType, fmt.Errorf("error decoding node configuration: %v", err)}
	}
	return &TunableClassifier{
		node: node,
		cfg: cfg,
		stats: new(vaas.StatsHolder),
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
	buf := vaas.NewSimpleBuffer(vaas.IntType)

	go func() {
		buf.SetMeta(parents[0].Freq())
		PerFrame(
			parents, ctx.Slice, buf, vaas.VideoType,
			vaas.ReadMultipleOptions{Stats: m.stats},
			func(idx int, data vaas.Data, buf vaas.DataWriter) error {
				im := data.(vaas.VideoData)[0]
				m.mu.Lock()
				t0 := time.Now()
				if m.stdin == nil {
					m.start()
				}
				header := make([]byte, 8)
				binary.BigEndian.PutUint32(header[0:4], uint32(im.Width))
				binary.BigEndian.PutUint32(header[4:8], uint32(im.Height))
				m.stdin.Write(header)
				m.stdin.Write(im.Bytes)
				line, err := m.rd.ReadString('\n')

				sample := vaas.StatsSample{}
				sample.Time.T = time.Now().Sub(t0)
				sample.Time.Count = 1
				m.stats.Add(sample)

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
				buf.Write(vaas.IntData{bestClass})
				return nil
			},
		)
	}()

	return buf
}

func (m *TunableClassifier) Close() {
	m.stdin.Close()
	m.cmd.Wait()
}

func (m *TunableClassifier) Stats() vaas.StatsSample {
	return m.stats.Get()
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
		Tune: func(node vaas.Node, gtlist []vaas.Data) [][2]string {
			var cfg TunableClassifierConfig
			vaas.JsonUnmarshal([]byte(node.Code), &cfg)
			dims := cfg.GetDims()

			var cfgs [][2]string
			for scaleCount := 0; scaleCount < len(dims); scaleCount++ {
				for depth := 1; scaleCount+depth <= len(dims); depth++ {
					cp := cfg
					cp.ScaleCount = scaleCount
					cp.Depth = depth
					desc := fmt.Sprintf("From %dx%d To %dx%d", dims[scaleCount][0], dims[scaleCount][1], dims[scaleCount+depth-1][0], dims[scaleCount+depth-1][1])
					cfgs = append(cfgs, [2]string{string(vaas.JsonMarshal(cp)), desc})
				}
			}
			return cfgs
		},
	}
}
