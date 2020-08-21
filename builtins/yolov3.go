package builtins

import (
	"../vaas"

	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Yolov3Config struct {
	InputSize [2]int
	ConfigPath string
	ModelPath string
	MetaPath string
}

func (cfg Yolov3Config) GetConfigPath() string {
	if cfg.ConfigPath == "" {
		return "cfg/yolov3.cfg"
	} else {
		return cfg.ConfigPath
	}
}

func (cfg Yolov3Config) GetModelPath() string {
	if cfg.ModelPath == "" {
		return "yolov3.weights"
	} else {
		return cfg.ModelPath
	}
}

func (cfg Yolov3Config) GetMetaPath() string {
	if cfg.MetaPath == "" {
		return "cfg/coco.data"
	} else {
		return cfg.MetaPath
	}
}

func CreateYolov3Cfg(fname string, cfg Yolov3Config, training bool) {
	// prepare configuration with this width/height
	configPath := cfg.GetConfigPath()
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join("darknet/", configPath)
	}
	bytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		panic(err)
	}
	file, err := os.Create(fname)
	if err != nil {
		panic(err)
	}
	for _, line := range strings.Split(string(bytes), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "width=") && cfg.InputSize[0] > 0 {
			line = fmt.Sprintf("width=%d", cfg.InputSize[0])
		} else if strings.HasPrefix(line, "height=") && cfg.InputSize[1] > 0 {
			line = fmt.Sprintf("height=%d", cfg.InputSize[1])
		} else if training && strings.HasPrefix(line, "batch=") {
			line = "batch=64"
		} else if training && strings.HasPrefix(line, "subdivisions=") {
			line = "subdivisions=8"
		}
		file.Write([]byte(line+"\n"))
	}
	file.Close()
}

type Yolov3 struct {
	node vaas.Node
	cmd *vaas.Cmd
	stdin io.WriteCloser
	rd *bufio.Reader
	cfg Yolov3Config
	cfgFname string
	mu sync.Mutex
	stats *vaas.StatsHolder
}

func NewYolov3(node vaas.Node) vaas.Executor {
	fmt.Println("new yolov3 on PID", os.Getpid())

	var cfgs []Yolov3Config
	err := json.Unmarshal([]byte(node.Code), &cfgs)
	if err != nil {
		return vaas.ErrorExecutor{node.DataType, fmt.Errorf("error decoding node configuration: %v", err)}
	}
	cfg := cfgs[0]

	cfgFname := fmt.Sprintf("%s/%d.cfg", os.TempDir(), rand.Int63())
	CreateYolov3Cfg(cfgFname, cfg, false)

	cmd := vaas.Command(
		"yolov3-run", vaas.CommandOptions{},
		"python3", "models/yolov3/run.py",
		cfgFname, cfg.GetModelPath(), cfg.GetMetaPath(),
	)

	return &Yolov3{
		node: node,
		cmd: cmd,
		stdin: cmd.Stdin(),
		rd: bufio.NewReader(cmd.Stdout()),
		cfg: cfg,
		cfgFname: cfgFname,
		stats: new(vaas.StatsHolder),
	}
}

func (m *Yolov3) Run(ctx vaas.ExecContext) vaas.DataBuffer {
	parents, err := GetParents(ctx, m.node)
	if err != nil {
		return vaas.GetErrorBuffer(m.node.DataType, fmt.Errorf("yolov3 error reading parents: %v", err))
	}
	buf := vaas.NewSimpleBuffer(vaas.DetectionType)

	go func() {
		// resize the input if possible
		// if not it's ok since yolov3 will handle resizing internally
		parent := parents[0]
		if vbufReader, ok := parent.(*vaas.VideoBufferReader); ok {
			vbufReader.Rescale([2]int{m.cfg.InputSize[0], m.cfg.InputSize[1]})
		}

		buf.SetMeta(parent.Freq())
		PerFrame(
			parents, ctx.Slice, buf, vaas.VideoType,
			vaas.ReadMultipleOptions{Stats: m.stats},
			func(idx int, data vaas.Data, buf vaas.DataWriter) error {
				im := data.(vaas.VideoData)[0]
				m.mu.Lock()
				t0 := time.Now()
				header := make([]byte, 8)
				binary.BigEndian.PutUint32(header[0:4], uint32(im.Width))
				binary.BigEndian.PutUint32(header[4:8], uint32(im.Height))
				m.stdin.Write(header)
				m.stdin.Write(im.Bytes)

				// ignore lines that don't start with "skyhook-yolov3"
				signature := "skyhook-yolov3"
				var line string
				var err error
				for {
					line, err = m.rd.ReadString('\n')
					if err != nil || strings.Contains(line, signature) {
						break
					}
				}

				sample := vaas.StatsSample{}
				sample.Time.T = time.Now().Sub(t0)
				sample.Time.Count = 1
				m.stats.Add(sample)

				m.mu.Unlock()

				if err != nil {
					return fmt.Errorf("error reading from yolov3 model: %v", err)
				}

				line = strings.TrimSpace(line[len(signature):])
				var detections vaas.DetectionFrame
				vaas.JsonUnmarshal([]byte(line), &detections)
				buf.Write(vaas.DetectionData{
					T: vaas.DetectionType,
					D: []vaas.DetectionFrame{detections},
				})
				return nil
			},
		)
	}()

	return buf
}

func (m *Yolov3) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stdin.Close()
	if m.cmd != nil {
		m.cmd.Wait()
		m.cmd = nil
	}
	os.Remove(m.cfgFname)
}

func (m *Yolov3) Stats() vaas.StatsSample {
	return m.stats.Get()
}

func init() {
	vaas.Executors["yolov3"] = vaas.ExecutorMeta{
		New: NewYolov3,
		Environment: &vaas.Environment{
			Template: "gpu",
			Requirements: map[string]int{
				"container": 1,
				"gpu": 1,
			},
		},
		Tune: func(node vaas.Node, gtlist []vaas.Data) [][2]string {
			var cfgs []Yolov3Config
			vaas.JsonUnmarshal([]byte(node.Code), &cfgs)

			var descs [][2]string
			for _, cfg := range cfgs {
				encoded := vaas.JsonMarshal([]Yolov3Config{cfg})
				descs = append(descs, [2]string{
					string(encoded),
					fmt.Sprintf("%dx%d", cfg.InputSize[0], cfg.InputSize[1]),
				})
			}
			return descs
		},
	}
}
