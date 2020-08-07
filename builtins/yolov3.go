package builtins

import (
	"../vaas"

	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Yolov3Config struct {
	InputSize [2]int
	ConfigPath string
	ModelPath string
}

func (cfg Yolov3Config) GetConfigPath() string {
	if cfg.ConfigPath == "" {
		return "darknet/cfg/yolov3.cfg"
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

type Yolov3 struct {
	cfg Yolov3Config
	node vaas.Node

	cfgFname string
	stdin io.WriteCloser
	rd *bufio.Reader
	cmd *vaas.Cmd
	width int
	height int
	mu sync.Mutex

	stats *vaas.StatsHolder
}

func NewYolov3(node vaas.Node) vaas.Executor {
	var cfg Yolov3Config
	err := json.Unmarshal([]byte(node.Code), &cfg)
	if err != nil {
		return vaas.ErrorExecutor{node.DataType, fmt.Errorf("error decoding node configuration: %v", err)}
	}
	return &Yolov3{
		node: node,
		cfg: cfg,
		stats: new(vaas.StatsHolder),
	}
}

func CreateYolov3Cfg(fname string, cfg Yolov3Config, dims [2]int, training bool) {
	// prepare configuration with this width/height
	bytes, err := ioutil.ReadFile(cfg.GetConfigPath())
	if err != nil {
		panic(err)
	}
	file, err := os.Create(fname)
	if err != nil {
		panic(err)
	}
	for _, line := range strings.Split(string(bytes), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "width=") {
			line = fmt.Sprintf("width=%d", dims[0])
		} else if strings.HasPrefix(line, "height=") {
			line = fmt.Sprintf("height=%d", dims[1])
		} else if training && strings.HasPrefix(line, "batch=") {
			line = "batch=64"
		} else if training && strings.HasPrefix(line, "subdivisions=") {
			line = "subdivisions=8"
		}
		file.Write([]byte(line+"\n"))
	}
	file.Close()
}

func (m *Yolov3) start() {
	m.cfgFname = fmt.Sprintf("%s/%d.cfg", os.TempDir(), rand.Int63())
	CreateYolov3Cfg(m.cfgFname, m.cfg, [2]int{m.width, m.height}, false)
	m.cmd = vaas.Command(
		"darknet",
		vaas.CommandOptions{F: func(cmd *exec.Cmd) {
			cmd.Dir = "darknet/"
		}},
		"./darknet", "detect", m.cfgFname, m.cfg.GetModelPath(), "-thresh", "0.1",
	)
	m.stdin = m.cmd.Stdin()
	m.rd = bufio.NewReader(m.cmd.Stdout())
	m.getLines()
}

func (m *Yolov3) getLines() []string {
	var output string
	for {
		line, err := m.rd.ReadString(':')
		if err != nil {
			panic(err)
		}
		output += line
		if strings.Contains(line, "Enter") {
			break
		}
	}
	return strings.Split(output, "\n")
}

func (m *Yolov3) Run(ctx vaas.ExecContext) vaas.DataBuffer {
	parents, err := GetParents(ctx, m.node)
	if err != nil {
		return vaas.GetErrorBuffer(m.node.DataType, fmt.Errorf("yolov3 error reading parents: %v", err))
	}
	buf := vaas.NewSimpleBuffer(vaas.DetectionType)

	parseLines := func(lines []string) []vaas.Detection {
		var boxes []vaas.Detection
		for i := 0; i < len(lines); i++ {
			if !strings.Contains(lines[i], "%") {
				continue
			}
			var box vaas.Detection
			parts := strings.Split(lines[i], ": ")
			box.Class = parts[0]
			score, _ := strconv.Atoi(strings.Trim(parts[1], "%"))
			box.Score = float64(score)/100
			for !strings.Contains(lines[i], "Bounding Box:") {
				i++
			}
			parts = strings.Split(strings.Split(lines[i], ": ")[1], ", ")
			if len(parts) != 4 {
				panic(fmt.Errorf("bad bbox line %s", lines[i]))
			}
			for _, part := range parts {
				kvsplit := strings.Split(part, "=")
				k := kvsplit[0]
				v, _ := strconv.Atoi(kvsplit[1])
				if k == "Left" {
					box.Left = v
				} else if k == "Top" {
					box.Top = v
				} else if k == "Right" {
					box.Right = v
				} else if k == "Bottom" {
					box.Bottom = v
				}
			}
			boxes = append(boxes, box)
		}
		return boxes
	}

	go func() {
		buf.SetMeta(parents[0].Freq())
		fname := fmt.Sprintf("%s/%d.jpg", os.TempDir(), rand.Int63())
		defer os.Remove(fname)
		PerFrame(
			parents, ctx.Slice, buf, vaas.VideoType,
			vaas.ReadMultipleOptions{Stats: m.stats},
			func(idx int, data vaas.Data, buf vaas.DataWriter) error {
				im := data.(vaas.VideoData)[0]
				if err := ioutil.WriteFile(fname, im.AsJPG(), 0644); err != nil {
					return err
				}
				m.mu.Lock()
				if m.stdin == nil {
					if m.cfg.InputSize[0] == 0 || m.cfg.InputSize[1] == 0 {
						m.width = (im.Width + 31) / 32 * 32
						m.height = (im.Height + 31) / 32 * 32
					} else {
						m.width = m.cfg.InputSize[0]
						m.height = m.cfg.InputSize[1]
					}
					log.Printf("[yolo] convert input %dx%d to %dx%d", im.Width, im.Height, m.width, m.height)
					m.start()
				}
				t0 := time.Now()
				m.stdin.Write([]byte(fname + "\n"))
				lines := m.getLines()
				boxes := parseLines(lines)

				sample := vaas.StatsSample{}
				sample.Time.T = time.Now().Sub(t0)
				sample.Time.Count = 1
				m.stats.Add(sample)

				m.mu.Unlock()
				ndata := vaas.DetectionData{
					T: vaas.DetectionType,
					D: []vaas.DetectionFrame{{
						Detections: boxes,
						CanvasDims: [2]int{im.Width, im.Height},
					}},
				}
				buf.Write(ndata)
				return nil
			},
		)
	}()

	return buf
}

func (m *Yolov3) Close() {
	m.stdin.Close()
	m.cmd.Wait()
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
	}
}
