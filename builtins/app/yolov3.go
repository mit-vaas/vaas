package builtins_app

import (
	"../../builtins"
	"../../app"
	"../../vaas"

	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Yolov3TrainJob struct {
	label string
	node *app.DBNode
	cfg builtins.Yolov3Config
	exportPath string

	lines *app.LinesBuffer
}

func NewYolov3TrainJob(label string, node *app.DBNode, cfg builtins.Yolov3Config, exportPath string) *Yolov3TrainJob {
	return &Yolov3TrainJob{
		label: label,
		node: node,
		cfg: cfg,
		exportPath: exportPath,
	}
}

// create obj.data, obj.names, {train,valid,test}.txt, and yolov3.cfg
func (j *Yolov3TrainJob) prepareConfigs() (string, error) {
	cfgDir, err := ioutil.TempDir("", "yolov3-train")
	if err != nil {
		return "", err
	}

	// yolov3.cfg
	builtins.CreateYolov3Cfg(filepath.Join(cfgDir, "yolov3.cfg"), j.cfg, true)

	// {train,valid,test}.txt
	dsPaths := [3]string{
		filepath.Join(cfgDir, "train.txt"),
		filepath.Join(cfgDir, "valid.txt"),
		filepath.Join(cfgDir, "test.txt"),
	}
	files, err := ioutil.ReadDir(j.exportPath)
	if err != nil {
		return "", err
	}
	var imagePaths []string
	for _, fi := range files {
		if !strings.HasSuffix(fi.Name(), ".jpg") {
			continue
		}
		imagePaths = append(imagePaths, filepath.Join(j.exportPath, fi.Name()))
	}
	rand.Shuffle(len(imagePaths), func(i, j int) {
		imagePaths[i], imagePaths[j] = imagePaths[j], imagePaths[i]
	})
	numVal := len(imagePaths)/10+1
	validPaths := strings.Join(imagePaths[0:numVal], "\n") + "\n"
	trainPaths := strings.Join(imagePaths[numVal:], "\n") + "\n"
	if err := ioutil.WriteFile(dsPaths[0], []byte(trainPaths), 0644); err != nil {
		return "", err
	}
	if err := ioutil.WriteFile(dsPaths[1], []byte(validPaths), 0644); err != nil {
		return "", err
	}
	if err := ioutil.WriteFile(dsPaths[2], []byte(validPaths), 0644); err != nil {
		return "", err
	}

	// obj.names
	namesPath := filepath.Join(cfgDir, "obj.names")
	if err := ioutil.WriteFile(namesPath, []byte("object"), 0644); err != nil {
		return "", err
	}

	// obj.data
	objDataTmpl := `
classes=1
train=%s
valid=%s
test=%s
names=%s
backup=%s
`
	objDataStr := fmt.Sprintf(objDataTmpl, dsPaths[0], dsPaths[1], dsPaths[2], namesPath, cfgDir)
	if err := ioutil.WriteFile(filepath.Join(cfgDir, "obj.data"), []byte(objDataStr), 0644); err != nil {
		return "", err
	}

	return cfgDir, nil
}

func (j *Yolov3TrainJob) Name() string {
	return j.label
}

func (j *Yolov3TrainJob) Type() string {
	return "cmd"
}

func (j *Yolov3TrainJob) Run(statusFunc func(string)) error {
	cfgDir, err := j.prepareConfigs()
	if err != nil {
		return err
	}

	statusFunc("Running")
	cmd := exec.Command(
		"./darknet", "detector", "train", "-map",
		filepath.Join(cfgDir, "obj.data"),
		filepath.Join(cfgDir, "yolov3.cfg"),
		"darknet53.conv.74",
	)
	cmd.Dir = "darknet/"
	stderr, err := cmd.StderrPipe()
	if err != nil {
		panic(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		rd := bufio.NewReader(stderr)
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				break
			}
			j.lines.Append("[stderr] " + strings.TrimSpace(line))
		}
		stderr.Close()
	}()
	// we simply pipe stderr to our saved lines, but for stdout we also look for the
	// mAP scores to determine when to stop training
	bestIterCh := make(chan int)
	go func() {
		rd := bufio.NewReader(stdout)
		var bestScore float64
		var bestAge int
		var bestIter int
		var curIter int
		for bestAge < 20 {
			line, err := rd.ReadString('\n')
			if err != nil {
				bestIter = -1
				break
			}
			j.lines.Append("[stdout] " + strings.TrimSpace(line))

			if strings.Contains(line, "mean average precision (mAP@0.50) = ") {
				line = strings.Split(line, "mean average precision (mAP@0.50) = ")[1]
				line = strings.Split(line, ",")[0]
				score := vaas.ParseFloat(line)
				if score > bestScore {
					bestScore = score
					bestAge = 0
					bestIter = curIter
					log.Printf("[yolov3-train] got new best mAP %v @ iteration %v", bestScore, bestIter)
				} else {
					bestAge++
					log.Printf("[yolov3-train] %d iterations with bad mAP", bestAge)
				}
			}

			if strings.Contains(line, "next mAP calculation at ") {
				line = strings.Split(line, "next mAP calculation at ")[1]
				line = strings.Split(line, " ")[0]
				curIter = vaas.ParseInt(line)
			}
		}
		cmd.Process.Kill()
		stdout.Close()
		bestIterCh <- bestIter
	}()
	cmd.Wait()
	bestIter := <- bestIterCh

	if bestIter == -1 {
		return fmt.Errorf("error running darknet")
	}

	// update the node configuration
	modelFname := fmt.Sprintf("yolov3_%d.weights", 1000*(bestIter/1000))
	cfg := builtins.Yolov3Config{
		InputSize: j.cfg.InputSize,
		MetaPath: filepath.Join(cfgDir, "obj.data"),
		ConfigPath: filepath.Join(cfgDir, "yolov3.cfg"),
		ModelPath: filepath.Join(cfgDir, modelFname),
	}

	var cfgs []builtins.Yolov3Config
	// refresh node just in case something changed
	node := app.GetNode(j.node.ID)
	if err := json.Unmarshal([]byte(node.Code), &cfgs); err != nil {
		cfgs = []builtins.Yolov3Config{cfg}
	} else {
		cfgs = append(cfgs, cfg)
	}
	cfgStr := string(vaas.JsonMarshal(cfgs))
	node.Update(&cfgStr, nil)

	return nil
}

func (j *Yolov3TrainJob) Detail() interface{} {
	return j.lines.Get()
}

func init() {
	http.HandleFunc("/yolov3/train", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		r.ParseForm()
		nodeID := vaas.ParseInt(r.PostForm.Get("node_id"))
		vectorID := vaas.ParseInt(r.PostForm.Get("vector_id"))
		width := vaas.ParseInt(r.PostForm.Get("width"))
		height := vaas.ParseInt(r.PostForm.Get("height"))
		configPath := r.PostForm.Get("config_path")
		vector := app.GetVector(vectorID)
		if vector == nil {
			w.WriteHeader(404)
			return
		}
		node := app.GetNode(nodeID)
		if node == nil {
			w.WriteHeader(404)
			return
		}

		exportPath := fmt.Sprintf("%s/export-%d-%d/", os.TempDir(), vector.ID, rand.Int63())
		if err := os.Mkdir(exportPath, 0755); err != nil {
			log.Printf("[yolov3] failed to export: could not mkdir %s: %v", exportPath, err)
			w.WriteHeader(400)
			return
		}

		exporter := app.ExportVector(vector.Vector, app.ExportOptions{
			Path: exportPath,
			Name: fmt.Sprintf("Export %s (for yolov3 training)", vector.Vector.Pretty()),
			YOLO: true,
		})

		cfg := builtins.Yolov3Config{
			InputSize: [2]int{width, height},
			ConfigPath: configPath,
		}

		go func() {
			err := app.RunJob(exporter)
			if err != nil {
				log.Printf("[yolov3] exiting since export job failed: %v", err)
				return
			}

			trainJob := NewYolov3TrainJob(
				fmt.Sprintf("Train YOLOv3 on %s", vector.Vector.Pretty()),
				node, cfg, exportPath,
			)
			err = app.RunJob(trainJob)
			if err != nil {
				log.Printf("[yolov3] train job failed: %v", err)
				return
			}
		}()
	})
}
