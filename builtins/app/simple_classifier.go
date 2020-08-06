package builtins_app

import (
	"../../builtins"
	"../../app"
	"../../vaas"

	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
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

		exportPath := fmt.Sprintf("%s/export-%d-%d/", os.TempDir(), series.ID, rand.Int63())
		if err := os.Mkdir(exportPath, 0755); err != nil {
			log.Printf("[simple-classifier] failed to export: could not mkdir %s", exportPath)
			w.WriteHeader(400)
			return
		}
		modelPath := fmt.Sprintf("models/simple-classifier-%d.h5", node.ID)

		exporter := app.ExportSeries(series, app.ExportOptions{
			Path: exportPath,
			Name: fmt.Sprintf("Export %s (for simple-classifier training)", series.Name),
		})
		go func() {
			err := app.RunJob(exporter)
			if err != nil {
				log.Printf("[simple-classifier] exiting since export job failed: %v", err)
				return
			}
			trainJob := app.NewCmdJob(
				fmt.Sprintf("Train Simple Classifier on %s", series.Name),
				"python3", "models/simple-classifier/train.py",
				exportPath, modelPath, "2",
			)
			err = app.RunJob(trainJob)
			if err != nil {
				log.Printf("[simple-classifier] exiting since train job failed: %v", err)
				return
			}
			cfg := builtins.SimpleClassifierConfig{modelPath}
			cfgStr := string(vaas.JsonMarshal(cfg))
			node.Update(&cfgStr, nil)
		}()
	})
}
