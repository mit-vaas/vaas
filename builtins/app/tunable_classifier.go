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
	"strconv"
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

		// auto-compute width, height, and number of classes
		item := &app.DBSeries{Series: series.SrcVector[0]}.ListItems()[0]
		width, height := item.Width, item.Height
		numClasses := 2
		for _, item := range series.ListItems() {
			data, err := item.Load(item.Slice).Reader().Read(item.Slice.Length())
			if err != nil {
				log.Printf("[tunable-classifier] error reading item at %s", item.Fname(0))
				w.WriteHeader(400)
				return
			}
			for _, cls := range data.(vaas.ClassData) {
				if numClasses <= cls {
					numClasses = cls+1
				}
			}
		}
		log.Printf("[tunable-classifier] automatically computed num_cls=%d, width=%d, height=%d", numClasses, width, height)

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
				exportPath, modelPath,
				strconv.Itoa(numClasses), strconv.Itoa(width), strconv.Itoa(height),
			)
			err = app.RunJob(trainJob)
			if err != nil {
				log.Printf("[tunable-classifier] exiting since train job failed: %v", err)
				return
			}
			cfg := builtins.TunableClassifierConfig{
				ModelPath: modelPath,
				MaxWidth: width,
				MaxHeight: height,
				NumClasses: numClasses,
				Depth: 1,
			}
			cfgStr := string(vaas.JsonMarshal(cfg))
			node.Update(&cfgStr, nil)
		}()
	})
}
