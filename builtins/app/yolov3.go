package builtins_app

import (
	"../../builtins"
	"../../app"
	"../../vaas"

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

func init() {
	// create obj.data, obj.names, {train,valid,test}.txt, and yolov3.cfg
	prepareConfigs := func(cfg builtins.Yolov3Config, exportPath string) (string, error) {
		cfgDir, err := ioutil.TempDir("", "yolov3-train")
		if err != nil {
			return "", err
		}

		// yolov3.cfg
		builtins.CreateYolov3Cfg(filepath.Join(cfgDir, "yolov3.cfg"), cfg, true)

		// {train,valid,test}.txt
		dsPaths := [3]string{
			filepath.Join(cfgDir, "train.txt"),
			filepath.Join(cfgDir, "valid.txt"),
			filepath.Join(cfgDir, "test.txt"),
		}
		files, err := ioutil.ReadDir(exportPath)
		if err != nil {
			return "", err
		}
		var imagePaths []string
		for _, fi := range files {
			if !strings.HasSuffix(fi.Name(), ".jpg") {
				continue
			}
			imagePaths = append(imagePaths, filepath.Join(exportPath, fi.Name()))
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

	http.HandleFunc("/yolov3/train", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		r.ParseForm()
		nodeID := vaas.ParseInt(r.PostForm.Get("node_id"))
		vectorID := vaas.ParseInt(r.PostForm.Get("vector_id"))
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
		//modelPath := fmt.Sprintf("models/yolov3-%d.weights", node.ID)

		exporter := app.ExportVector(vector.Vector, app.ExportOptions{
			Path: exportPath,
			Name: fmt.Sprintf("Export %s (for yolov3 training)", vector.Vector.Pretty()),
			YOLO: true,
		})

		var cfgs []builtins.Yolov3Config
		vaas.JsonUnmarshal([]byte(node.Code), &cfgs)
		cfgIdx := len(cfgs)-1

		go func() {
			err := app.RunJob(exporter)
			if err != nil {
				log.Printf("[yolov3] exiting since export job failed: %v", err)
				return
			}

			cfgDir, err := prepareConfigs(cfgs[cfgIdx], exportPath)
			if err != nil {
				log.Printf("[yolov3] failed to prepare configs: %v", err)
				return
			}

			trainJob := app.NewCmdJob(
				fmt.Sprintf("Train YOLOv3 on %s", vector.Vector.Pretty()),
				"timeout", "2h",
				"./darknet", "detector", "train",
				filepath.Join(cfgDir, "obj.data"),
				filepath.Join(cfgDir, "yolov3.cfg"),
				"darknet53.conv.74",
			)
			trainJob.F = func(cmd *exec.Cmd) {
				cmd.Dir = "darknet/"
			}
			err = app.RunJob(trainJob)
			if err != nil {
				log.Printf("[yolov3] exiting since train job failed: %v", err)
				return
			}

			// TODO: loop through them and find the best model?
		}()
	})
}
