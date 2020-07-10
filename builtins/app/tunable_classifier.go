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
	http.HandleFunc("/tunable-classifier/train", func(w http.ResponseWriter, r *http.Request) {
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
		series.Load()
		node := app.GetNode(nodeID)
		if node == nil {
			w.WriteHeader(404)
			return
		}

		var refs [][]app.DataRef
		for _, s := range series.SrcVector {
			refs = append(refs, []app.DataRef{app.DataRef{
				Series: &s,
			}})
		}
		refs = append(refs, []app.DataRef{app.DataRef{
			Series: &series.Series,
		}})
		exportPath := fmt.Sprintf("%s/export-%d-%d/", os.TempDir(), series.ID, rand.Int63())
		if err := os.Mkdir(exportPath, 0755); err != nil {
			log.Printf("[tunable-classifier] failed to export: could not mkdir %s", exportPath)
			w.WriteHeader(400)
			return
		}
		modelPath := fmt.Sprintf("models/tunable-classifier-%d.h5", node.ID)

		exporter := app.NewExporter(refs, app.ExportOptions{
			Path: exportPath,
			Name: fmt.Sprintf("Export %s (for tunable-classifier training)", series.Name),
		})
		go func() {
			err := app.RunJob(exporter)
			if err != nil {
				log.Printf("[tunable-classifier] exiting since export job failed: %v", err)
				return
			}
			trainJob := app.NewCmdJob(
				fmt.Sprintf("Train Tunable Classifier on %s", series.Name),
				"python3", "models/tunable-classifier/train.py",
				exportPath, modelPath, "2", "1280", "720",
			)
			err = app.RunJob(trainJob)
			if err != nil {
				log.Printf("[tunable-classifier] exiting since train job failed: %v", err)
				return
			}
			cfg := builtins.TunableClassifierConfig{
				ModelPath: modelPath,
				MaxWidth: 1280,
				MaxHeight: 720,
				NumClasses: 2,
				Depth: 1,
			}
			cfgStr := string(vaas.JsonMarshal(cfg))
			node.Update(&cfgStr, nil)
		}()
	})
}
