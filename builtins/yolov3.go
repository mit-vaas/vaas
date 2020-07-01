package builtins

import (
	"../vaas"

	"bufio"
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
	CanvasSize [2]int
	InputSize [2]int
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

	stats vaas.StatsSample
}

func NewYolov3(node vaas.Node) vaas.Executor {
	var cfg Yolov3Config
	vaas.JsonUnmarshal([]byte(node.Code), &cfg)
	return &Yolov3{
		node: node,
		cfg: cfg,
	}
}

func (m *Yolov3) start() {
	// prepare configuration with this width/height
	bytes, err := ioutil.ReadFile("darknet/cfg/yolov3.cfg")
	if err != nil {
		panic(err)
	}
	m.cfgFname = fmt.Sprintf("%s/%d.cfg", os.TempDir(), rand.Int63())
	file, err := os.Create(m.cfgFname)
	if err != nil {
		panic(err)
	}
	for _, line := range strings.Split(string(bytes), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "width=") {
			line = fmt.Sprintf("width=%d", m.width)
		} else if strings.HasPrefix(line, "height=") {
			line = fmt.Sprintf("height=%d", m.height)
		}
		file.Write([]byte(line+"\n"))
	}
	file.Close()
	m.cmd = vaas.Command(
		"darknet",
		vaas.CommandOptions{F: func(cmd *exec.Cmd) {
			cmd.Dir = "darknet/"
		}},
		"./darknet", "detect", m.cfgFname, "yolov3.weights", "-thresh", "0.1",
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
		PerFrame(parents, ctx.Slice, buf, vaas.VideoType, func(idx int, data vaas.Data, buf vaas.DataWriter) error {
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
			m.stats = m.stats.Add(vaas.StatsSample{
				Time: time.Now().Sub(t0),
				Count: 1,
			})
			m.mu.Unlock()
			if m.cfg.CanvasSize[0] != 0 && m.cfg.CanvasSize[1] != 0 {
				scale := [2]float64{
					float64(m.cfg.CanvasSize[0]) / float64(im.Width),
					float64(m.cfg.CanvasSize[1]) / float64(im.Height),
				}
				boxes = vaas.ResizeDetections([][]vaas.Detection{boxes}, scale)[0]
			}
			buf.Write(vaas.DetectionData{boxes})
			return nil
		})
	}()

	return buf
}

func (m *Yolov3) Close() {
	m.stdin.Close()
	m.cmd.Wait()
}

func (m *Yolov3) Stats() vaas.StatsSample {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stats
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
