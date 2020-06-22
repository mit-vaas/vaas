package builtins

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

type Yolov3 struct {
	cfgFname string
	stdin io.WriteCloser
	rd *bufio.Reader
	cmd *skyhook.Cmd
	mu sync.Mutex
}

func NewYolov3(node *skyhook.Node, query *skyhook.Query) skyhook.Executor {
	return &Yolov3{}
}

func (m *Yolov3) start(width int, height int) {
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
			line = fmt.Sprintf("width=%d", width)
		} else if strings.HasPrefix(line, "height=") {
			line = fmt.Sprintf("height=%d", height)
		}
		file.Write([]byte(line+"\n"))
	}
	file.Close()
	m.cmd = skyhook.Command(
		"darknet",
		skyhook.CommandOptions{F: func(cmd *exec.Cmd) {
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

func (m *Yolov3) Run(parents []skyhook.DataReader, slice skyhook.Slice) skyhook.DataBuffer {
	buf := skyhook.NewSimpleBuffer(skyhook.DetectionType, parents[0].Freq())

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
		PerFrame(parents, slice, buf, skyhook.VideoType, func(idx int, data skyhook.Data, buf skyhook.DataWriter) error {
			im := data.(skyhook.VideoData)[0]
			if err := ioutil.WriteFile(fname, im.AsJPG(), 0644); err != nil {
				return err
			}
			m.mu.Lock()
			if m.stdin == nil {
				m.start(im.Width, im.Height)
			}
			m.stdin.Write([]byte(fname + "\n"))
			lines := m.getLines()
			boxes := parseLines(lines)
			m.mu.Unlock()
			buf.Write(skyhook.DetectionData{boxes})
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
	skyhook.Executors["yolov3"] = NewYolov3
}
