package models

import (
	"../skyhook"

	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

type Yolov3Config struct {
	Threshold float64
}

type Yolov3 struct {
	threshold float64
	stdin io.WriteCloser
	rd *bufio.Reader
	cmd *exec.Cmd
	mu sync.Mutex
}

func NewYolov3(cfgBytes []byte) skyhook.Executor {
	type Config struct {
		Threshold float64
	}
	var cfg Config
	skyhook.JsonUnmarshal(cfgBytes, &cfg)

	cmd := exec.Command("./darknet", "detect", "cfg/yolov3.cfg", "yolov3.weights", "-thresh", fmt.Sprintf("%v", cfg.Threshold))
	cmd.Dir = "darknet/"
	stdin, err := cmd.StdinPipe()
	if err != nil {
		panic(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		panic(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	go skyhook.PrintStderr("darknet", stderr, false)
	rd := bufio.NewReader(stdout)
	return &Yolov3{
		threshold: cfg.Threshold,
		stdin: stdin,
		rd: rd,
		cmd: cmd,
	}
}

func (m *Yolov3) Run(parents []*skyhook.BufferReader, slice skyhook.ClipSlice) *skyhook.LabelBuffer {
	buf := skyhook.NewLabelBuffer(skyhook.DetectionType)

	getLines := func() []string {
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

	parseLines := func(lines []string) []skyhook.Detection {
		var boxes []skyhook.Detection
		for i := 0; i < len(lines); i++ {
			if !strings.Contains(lines[i], "%") {
				continue
			}
			var box skyhook.Detection
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
		fname := fmt.Sprintf("%s/%d.jpg", os.TempDir(), rand.Int63())
		defer os.Remove(fname)
		PerFrame(parents, slice, buf, skyhook.VideoType, func(idx int, data skyhook.Data, buf *skyhook.LabelBuffer) error {
			im := data.Images[0]
			if err := ioutil.WriteFile(fname, im.AsJPG(), 0644); err != nil {
				return err
			}
			m.mu.Lock()
			m.stdin.Write([]byte(fname + "\n"))
			lines := getLines()
			boxes := parseLines(lines)
			m.mu.Unlock()
			buf.Write(skyhook.Data{
				Type: skyhook.DetectionType,
				Detections: [][]skyhook.Detection{boxes},
			})
			return nil
		})
	}()

	return buf
}

func (m *Yolov3) Close() {
	m.stdin.Close()
	m.cmd.Wait()
}

func init() {
	skyhook.Models["yolov3"] = NewYolov3
}
