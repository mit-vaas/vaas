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
	"strings"
	"sync"
)

func init() {
	// ensure detections for at least N nframes-second slices are computed
	preprocess := func(nframes, n int, detectionNode *app.DBNode, detectionSeries *app.DBSeries, vector []*app.DBSeries) error {
		// count how many new slices we need to collect outputs for
		existingSet := make(map[[2]int]bool) // set of (segment ID, start idx)
		for _, item := range detectionSeries.ListItems() {
			start := (item.Slice.Start + nframes-1) / nframes * nframes
			end := item.Slice.End / nframes * nframes
			for idx := start; idx < end; idx += nframes {
				existingSet[[2]int{item.Slice.Segment.ID, idx}] = true
			}
		}
		needed := n - len(existingSet)

		if needed == 0 {
			return nil
		}

		// persist detections for that many new slices by applying the query repeatedly
		sampler := func() *vaas.Slice {
			slice := app.DBTimeline{Timeline: vector[0].Timeline}.Uniform(nframes)
			return &slice
		}
		query := app.GetQuery(detectionNode.QueryID)
		query.Load()
		opts := vaas.ExecOptions{
			Outputs: [][]vaas.Parent{{vaas.Parent{
				Type: vaas.NodeParent,
				NodeID: detectionNode.ID,
			}}},
			NoSelector: true,
		}
		var mu sync.Mutex
		var execErr error
		log.Printf("[selfsupervised-tracker] collecting detection outputs for %d slices", needed)
		stream := app.NewExecStream(query, vector, sampler, 4, opts, func(slice vaas.Slice, outputs [][]vaas.DataReader, err error) {
			if err == nil || strings.Contains(err.Error(), "sample error") {
				return
			}
			mu.Lock()
			execErr = err
			mu.Unlock()
		})
		stream.Get(needed)
		return execErr
	}

	http.HandleFunc("/selfsupervised-tracker/train", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		r.ParseForm()
		nodeID := vaas.ParseInt(r.PostForm.Get("node_id"))
		seriesID := vaas.ParseInt(r.PostForm.Get("series_id"))
		node := app.GetNode(nodeID)
		if node == nil {
			w.WriteHeader(404)
			return
		}
		series := app.GetSeries(seriesID)
		if series == nil {
			w.WriteHeader(404)
			return
		}
		series.Load()

		// get series for the detection node parent
		if len(node.Parents) != 1 || node.Parents[0].Type != vaas.NodeParent {
			log.Printf("[selfsupervised-tracker train] tracker node must have single detection node parent")
			w.WriteHeader(400)
			return
		}
		vector := []*app.DBSeries{series}
		detectionNode := app.GetNode(node.Parents[0].NodeID)
		vn := app.GetOrCreateVNode(detectionNode, vector)
		vn.EnsureSeries()
		detectionSeries := &app.DBSeries{Series: *vn.Series}

		go func() {
			// preprocess
			nframes := 30*vaas.FPS
			preprocessJob := app.JobFunc("selfsupervised-tracker-preprocess", "cmd", func() (interface{}, error) {
				err := preprocess(nframes, 1000, detectionNode, detectionSeries, vector)
				return []string{}, err
			})
			err := app.RunJob(preprocessJob)
			if err != nil {
				log.Printf("[selfsupervised-tracker train] exiting since preprocess job failed: %v", err)
				return
			}

			// determine freq to export at
			// we want to set this to ensure the video and detections are aligned
			// use the smallest freq of the detections
			var exportFreq int = -1
			for _, item := range detectionSeries.ListItems() {
				if exportFreq == -1 || item.Freq < exportFreq {
					exportFreq = item.Freq
				}
			}

			// export
			trainVector := []*app.DBSeries{}
			trainVector = append(trainVector, vector...)
			trainVector = append(trainVector, detectionSeries)
			var slices []vaas.Slice
			for _, item := range detectionSeries.ListItems() {
				if item.Slice.Length() < nframes {
					continue
				}
				slices = append(slices, item.Slice)
			}
			exportPath := fmt.Sprintf("%s/export-%d-%d/", os.TempDir(), node.ID, rand.Int63())
			if err := os.Mkdir(exportPath, 0755); err != nil {
				log.Printf("[selfsupervised-tracker train] failed to export: could not mkdir %s", exportPath)
				return
			}
			exporter := app.NewExporter(trainVector, slices, app.ExportOptions{
				Path: exportPath,
				Name: fmt.Sprintf("Export %s (for selfsupervised-tracker training)", detectionSeries.Name),
				Freq: exportFreq,
			})
			err = app.RunJob(exporter)
			if err != nil {
				log.Printf("[selfsupervised-tracker train] exiting since export job failed: %v", err)
				return
			}
			modelPath := fmt.Sprintf("models/selfsupervised-tracker-%d", node.ID)

			// train
			trainJob := app.NewCmdJob(
				fmt.Sprintf("Train Self-Supervised Tracker on %s", detectionSeries.Name),
				"python3", "models/selfsupervised-tracker/train.py",
				exportPath, modelPath, "1",
			)
			err = app.RunJob(trainJob)
			if err != nil {
				log.Printf("[selfsupervised-tracker train] exiting since train job failed: %v", err)
				return
			}

			cfg := builtins.SelfSupervisedTrackerConfig{
				ModelPath: modelPath,
			}
			cfgStr := string(vaas.JsonMarshal(cfg))
			node.Update(&cfgStr, nil)
		}()
	})
}
