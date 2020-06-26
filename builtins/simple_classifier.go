package builtins

import (
	"../skyhook"

	"bufio"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
)

type SimpleClassifierConfig struct {
	ModelPath string
}

type SimpleClassifier struct {
	node *skyhook.Node
	cfg SimpleClassifierConfig
	stdin io.WriteCloser
	rd *bufio.Reader
	cmd *skyhook.Cmd
	mu sync.Mutex
}

func NewSimpleClassifier(node *skyhook.Node) skyhook.Executor {
	var cfg SimpleClassifierConfig
	skyhook.JsonUnmarshal([]byte(node.Code), &cfg)
	return &SimpleClassifier{
		node: node,
		cfg: cfg,
	}
}

func (m *SimpleClassifier) Run(ctx skyhook.ExecContext) skyhook.DataBuffer {
	parents, err := GetParents(ctx, m.node)
	if err != nil {
		return skyhook.GetErrorBuffer(m.node.DataType, fmt.Errorf("simple-classifier error reading parents: %v", err))
	}
	buf := skyhook.NewSimpleBuffer(skyhook.DetectionType)

	go func() {
		buf.SetMeta(parents[0].Freq())
		PerFrame(parents, ctx.Slice, buf, skyhook.VideoType, func(idx int, data skyhook.Data, buf skyhook.DataWriter) error {
			//im := data.(skyhook.VideoData)[0]
			// ...
			return nil
		})
	}()

	return buf
}

func (m *SimpleClassifier) Close() {
	m.stdin.Close()
	m.cmd.Wait()
}

func init() {
	skyhook.Executors["simple-classifier"] = NewSimpleClassifier

	http.HandleFunc("/simple-classifier/train", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		r.ParseForm()
		nodeID := skyhook.ParseInt(r.PostForm.Get("node_id"))
		seriesID := skyhook.ParseInt(r.PostForm.Get("series_id"))
		series := skyhook.GetSeries(seriesID)
		if series == nil {
			w.WriteHeader(404)
			return
		}
		series.Load()
		node := skyhook.GetNode(nodeID)
		if node == nil {
			w.WriteHeader(404)
			return
		}

		var refs [][]skyhook.DataRef
		for _, s := range series.SrcVector {
			refs = append(refs, []skyhook.DataRef{skyhook.DataRef{
				Series: s,
			}})
		}
		refs = append(refs, []skyhook.DataRef{skyhook.DataRef{
			Series: series,
		}})
		exportPath := fmt.Sprintf("%s/export-%d-%d/", os.TempDir(), series.ID, rand.Int63())
		if err := os.Mkdir(exportPath, 0755); err != nil {
			log.Printf("[simple-classifier] failed to export: could not mkdir %s", exportPath)
			w.WriteHeader(400)
			return
		}
		modelPath := fmt.Sprintf("models/simple-classifier-%d.h5", node.ID)

		exporter := skyhook.NewExporter(refs, skyhook.ExportOptions{
			Path: exportPath,
			Name: fmt.Sprintf("Export %s (for simple-classifier training)", series.Name),
		})
		go func() {
			err := skyhook.RunJob(exporter)
			if err != nil {
				log.Printf("[simple-classifier] exiting since export job failed: %v", err)
				return
			}
			trainJob := skyhook.NewCmdJob(
				fmt.Sprintf("Train Simple Classifier on %s", series.Name),
				"python3.6", "models/simple-classifier/train.py",
				exportPath, modelPath, "2",
			)
			err = skyhook.RunJob(trainJob)
			if err != nil {
				log.Printf("[simple-classifier] exiting since train job failed: %v", err)
				return
			}
			cfg := SimpleClassifierConfig{modelPath}
			cfgStr := string(skyhook.JsonMarshal(cfg))
			node.Update(&cfgStr, nil)
		}()
	})
}
