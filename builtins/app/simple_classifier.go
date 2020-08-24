package builtins_app

import (
	"../../builtins"
	"../../app"
	"../../vaas"

	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
)

func init() {
	http.HandleFunc("/simple-classifier/train", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		r.ParseForm()
		nodeID := vaas.ParseInt(r.PostForm.Get("node_id"))
		seriesID := vaas.ParseInt(r.PostForm.Get("series_id"))
		numClasses := vaas.ParseInt(r.PostForm.Get("num_classes"))
		width := vaas.ParseInt(r.PostForm.Get("width"))
		height := vaas.ParseInt(r.PostForm.Get("height"))
		series := app.GetSeries(seriesID)
		if series == nil {
			w.WriteHeader(404)
			return
		}
		node := app.GetNode(nodeID)
		if node == nil {
			w.WriteHeader(404)
			return
		}

		if width == 0 && height == 0 {
			// set width/height automatically by using dimensions of an arbitrary item in the series
			series.Load()
			item := &app.DBSeries{Series: series.SrcVector[0]}.ListItems()[0]
			width = item.Width
			height = item.Height
			log.Printf("[simple-classifier] node %s: automatically set width=%d,height=%d", node.Name, width, height)
		}

		exportPath := fmt.Sprintf("%s/export-%d-%d/", os.TempDir(), series.ID, rand.Int63())
		if err := os.Mkdir(exportPath, 0755); err != nil {
			log.Printf("[simple-classifier] node %s: failed to export: could not mkdir %s", node.Name, exportPath)
			w.WriteHeader(400)
			return
		}
		modelPath := fmt.Sprintf("./node-data/simple-classifier-%d-%d.h5", node.ID, rand.Int63())

		exporter := app.ExportSeries(series, app.ExportOptions{
			Path: exportPath,
			Name: fmt.Sprintf("Export %s (for simple-classifier training)", series.Name),
		})
		go func() {
			err := app.RunJob(exporter)
			if err != nil {
				log.Printf("[simple-classifier] node %s: exiting since export job failed: %v", node.Name, err)
				return
			}
			trainJob := app.NewCmdJob(
				fmt.Sprintf("Train Simple Classifier on %s", series.Name),
				"python3", "models/simple-classifier/train.py",
				exportPath, modelPath, strconv.Itoa(numClasses), strconv.Itoa(width), strconv.Itoa(height),
			)
			err = app.RunJob(trainJob)
			if err != nil {
				log.Printf("[simple-classifier] node %s: exiting since train job failed: %v", node.Name, err)
				return
			}
			cfg := builtins.SimpleClassifierConfig{
				ModelPath: modelPath,
				NumClasses: numClasses,
				InputSize: [2]int{width, height},
			}

			var cfgs []builtins.SimpleClassifierConfig
			if err := json.Unmarshal([]byte(node.Code), &cfgs); err != nil {
				cfgs = []builtins.SimpleClassifierConfig{cfg}
			} else {
				cfgs = append(cfgs, cfg)
			}
			cfgStr := string(vaas.JsonMarshal(cfgs))
			node.Update(&cfgStr, nil)
		}()
	})
}
