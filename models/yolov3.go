package models

import (
	"../skyhook"

	"bufio"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type Yolov3Config struct {
	Threshold float64
}

func init() {
	skyhook.Models["yolov3"] = func(cfgBytes []byte, parents [][]*skyhook.BufferReader, slices []skyhook.ClipSlice) []*skyhook.LabelBuffer {
		var cfg Yolov3Config
		skyhook.JsonUnmarshal(cfgBytes, &cfg)

		buffers := make([]*skyhook.LabelBuffer, len(slices))
		for i := range buffers {
			buffers[i] = skyhook.NewLabelBuffer(skyhook.DetectionType)
		}

		go func() {
			c := exec.Command("./darknet", "detect", "cfg/yolov3.cfg", "yolov3.weights", "-thresh", fmt.Sprintf("%v", cfg.Threshold))
			c.Dir = "darknet/"
			stdin, err := c.StdinPipe()
			if err != nil {
				panic(err)
			}
			stderr, err := c.StderrPipe()
			if err != nil {
				panic(err)
			}
			stdout, err := c.StdoutPipe()
			if err != nil {
				panic(err)
			}
			if err := c.Start(); err != nil {
				panic(err)
			}
			go skyhook.PrintStderr("darknet", stderr, false)
			r := bufio.NewReader(stdout)

			getLines := func() []string {
				var output string
				for {
					line, err := r.ReadString(':')
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

			fname := fmt.Sprintf("%s/%d.jpg", os.TempDir(), rand.Int63())
			defer os.Remove(fname)
			PerFrame(parents, slices, buffers, func(im skyhook.Image, outBuf *skyhook.LabelBuffer) error {
				if err := ioutil.WriteFile(fname, im.AsJPG(), 0644); err != nil {
					return err
				}
				stdin.Write([]byte(fname + "\n"))
				lines := getLines()
				boxes := parseLines(lines)
				outBuf.Write(skyhook.Data{
					Type: skyhook.DetectionType,
					Detections: [][]skyhook.Detection{boxes},
				})
				return nil
			})

			stdin.Close()
			c.Wait()
		}()

		return buffers
	}
}
