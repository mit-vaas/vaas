package app

import (
	"../vaas"

	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
)

type ExportFormat string
const (
	CSVFormat ExportFormat = "csv"
	SQLiteFormat ExportFormat = "sqlite"
)

type ExportOptions struct {
	// path to write exports
	// should be a directory that exists
	// ignored if Render is true
	Path string

	// data that can be represented in this format will be converted
	// the other options override this format
	Format ExportFormat

	// detections will be output in YOLOv3 txt format
	YOLO bool

	// tracks will be output on MOT txt format
	MOT bool

	// Instead of exporting, render them as one video
	Render bool

	// Outputs should be per-frame not sequences.
	// So if we get a slice with two frames we should split it up.
	PerFrame bool

	// Custom job name
	Name string

	// export all items at this freq
	Freq int
}

type Exporter struct {
	refs [][]ConcreteRef
	opts ExportOptions

	l []string
	mu sync.Mutex
}

// Exporter from groups of ConcreteRefs that all have the same slice.
func NewExporter(refs [][]ConcreteRef, opts ExportOptions) *Exporter {
	return &Exporter{refs: refs, opts: opts}
}

func NewExporterDataRefs(refs [][]DataRef, opts ExportOptions) *Exporter {
	var crefs [][]ConcreteRef
	EnumerateDataRefs(refs, func(slice vaas.Slice, group []ConcreteRef) error {
		for i := range group {
			group[i].Slice = slice
		}
		crefs = append(crefs, group)
		return nil
	})
	return NewExporter(crefs, opts)
}

func (e *Exporter) Name() string {
	if e.opts.Name != "" {
		return e.opts.Name
	}
	return "Export"
}

func (e *Exporter) Type() string {
	return "cmd"
}

func encodeDetectionsAsYOLO(data vaas.Data) []byte {
	if data.Length() != 1 {
		panic(fmt.Errorf("encodeDetectionsAsYOLO: length of detections must be 1"))
	}
	df := data.(vaas.DetectionData).D[0]
	var lines []string
	for _, d := range df.Detections {
		cx := float64(d.Left+d.Right)/2 / float64(df.CanvasDims[0])
		cy := float64(d.Top+d.Bottom)/2 / float64(df.CanvasDims[1])
		w := float64(d.Right-d.Left) / float64(df.CanvasDims[0])
		h := float64(d.Bottom-d.Top) / float64(df.CanvasDims[1])
		line := fmt.Sprintf("0 %v %v %v %v", cx, cy, w, h)
		lines = append(lines, line)
	}
	lines = append(lines, "")
	return []byte(strings.Join(lines, "\n"))
}

func (e *Exporter) Run(statusFunc func(string)) error {
	statusFunc("Exporting")

	exportVideo := func(slice vaas.Slice, ref ConcreteRef, prefix string) error {
		if ref.Item == nil && ref.Series == nil {
			return fmt.Errorf("error exporting video: video must be from item not encoded")
		}
		var series *DBSeries
		var item *vaas.Item
		if ref.Item != nil {
			series = GetSeries(ref.Item.Series.ID)
			item = ref.Item
		} else {
			series = GetSeries(ref.Series.ID)
			dbitem := series.GetItem(slice)
			if dbitem != nil {
				item = &dbitem.Item
			}
		}
		if slice.Length() == 1 {
			buf, err := series.RequireData(slice)
			if err != nil {
				return err
			}
			rd := buf.Reader()
			defer rd.Close()
			data, err := rd.Read(1)
			if err != nil {
				return err
			}
			im := data.(vaas.VideoData)[0]
			if err := ioutil.WriteFile(prefix + ".jpg", im.AsJPG(), 0644); err != nil {
				return err
			}
		} else if item != nil && item.Slice.Equals(slice) && (e.opts.Freq == 0 || e.opts.Freq == item.Freq) && item.Format == "mp4" {
			// copy the file contents directly when possible
			srcFile, err := os.Open(item.Fname(0))
			if err != nil {
				return err
			}
			defer srcFile.Close()
			dstFile, err := os.Create(prefix + ".mp4")
			if err != nil {
				return err
			}
			if _, err := io.Copy(dstFile, srcFile); err != nil {
				return err
			}
		} else {
			buf, err := series.RequireData(slice)
			if err != nil {
				return err
			}
			rd := buf.Reader().(*vaas.VideoBufferReader)
			if e.opts.Freq != 0 {
				rd.Resample(e.opts.Freq / rd.Freq())
			}
			defer rd.Close()
			file, err := os.Create(prefix + ".mp4")
			if err != nil {
				return err
			}
			defer file.Close()
			if err := rd.ReadMP4(file); err != nil {
				return err
			}
		}
		return nil
	}

	exportOther := func(slice vaas.Slice, ref ConcreteRef, prefix string) error {
		var data vaas.Data
		var freq int
		if ref.Item != nil || ref.Series != nil {
			var buf vaas.DataBuffer
			if ref.Item != nil {
				buf = ref.Item.Load(slice)
			} else if ref.Series != nil {
				var err error
				series := &DBSeries{Series: *ref.Series}
				buf, err = series.RequireData(slice)
				if err != nil {
					return err
				}
			}
			var err error
			data, err = buf.Reader().Read(slice.Length())
			if err != nil {
				return fmt.Errorf("error reading buffer: %v", err)
			}
			freq = ref.Item.Freq
		} else {
			data = ref.Data.Slice(slice.Start - ref.Slice.Start, slice.End - ref.Slice.End)
			freq = 1
		}
		if e.opts.Freq != 0 {
			data = vaas.AdjustDataFreq(data, slice.Length(), freq, e.opts.Freq)
		}
		var encoded []byte
		var ext string
		if e.opts.YOLO && data.Type() == vaas.DetectionType {
			encoded = encodeDetectionsAsYOLO(data)
			ext = "txt"
		} else {
			encoded = data.Encode()
			ext = "json"
		}
		fname := prefix + "." + ext
		if err := ioutil.WriteFile(fname, encoded, 0644); err != nil {
			return fmt.Errorf("error writing encoded data to %s: %v", fname, err)
		}
		return nil
	}

	exportGroup := func(group []ConcreteRef) error {
		slice := group[0].Slice
		prefix := fmt.Sprintf("%s/%d_%d_%d", e.opts.Path, slice.Segment.ID, slice.Start, slice.End)
		for i, ref := range group {
			// decide whether to name files N_0.abc, N_1.xyz N.abc, N.xyz
			var curPrefix string
			if e.opts.YOLO {
				curPrefix = prefix
			} else {
				curPrefix = prefix + fmt.Sprintf("_%d", i)
			}

			var err error
			if ref.Type() == vaas.VideoType {
				err = exportVideo(slice, ref, curPrefix)
			} else {
				err = exportOther(slice, ref, curPrefix)
			}
			if err != nil {
				e.mu.Lock()
				e.l = append(e.l, fmt.Sprintf("error exporting %s: %v", prefix, err))
				e.mu.Unlock()
				return err
			}
		}
		e.mu.Lock()
		e.l = append(e.l, prefix)
		e.mu.Unlock()
		return nil
	}

	var nextIdx int = 0
	donech := make(chan error)

	nthreads := runtime.NumCPU()
	log.Printf("[job %s] exporting %d slices", e.Name(), len(e.refs))
	for i := 0; i < nthreads; i++ {
		go func() {
			var err error
			for {
				e.mu.Lock()
				if nextIdx >= len(e.refs) {
					e.mu.Unlock()
					break
				}
				idx := nextIdx
				nextIdx++
				e.mu.Unlock()
				err = exportGroup(e.refs[idx])
				if err != nil {
					break
				}
			}
			donech <- err
		}()
	}
	var err error
	for i := 0; i < nthreads; i++ {
		curErr := <- donech
		if curErr != nil {
			err = curErr
		}
	}
	return err
}

func (e *Exporter) Detail() interface{} {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.l
}

// Creates a job to export a series along with series from its SrcVector.
func ExportSeries(series *DBSeries, opts ExportOptions) *Exporter {
	series.Load()
	var refs [][]ConcreteRef
	for _, item := range series.ListItems() {
		var cur []ConcreteRef
		for _, s := range series.SrcVector {
			s_ := s
			cur = append(cur, ConcreteRef{
				Slice: item.Slice,
				Series: &s_,
			})
		}
		item_ := item.Item
		cur = append(cur, ConcreteRef{
			Slice: item.Slice,
			Item: &item_,
		})
		refs = append(refs, cur)
	}
	return NewExporter(refs, opts)
}

func ExportVector(vector []*DBSeries, opts ExportOptions) *Exporter {
	var refs [][]DataRef
	for _, series := range vector {
		s := series.Series
		refs = append(refs, []DataRef{{
			Series: &s,
		}})
	}
	return NewExporterDataRefs(refs, opts)
}

func init() {
	http.HandleFunc("/series/export", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		r.ParseForm()
		seriesID := vaas.ParseInt(r.PostForm.Get("series_id"))
		series := GetSeries(seriesID)
		if series == nil {
			http.Error(w, "no such series", 404)
			return
		}
		exportPath := fmt.Sprintf("%s/export-%d-%d/", os.TempDir(), series.ID, rand.Int63())
		if err := os.Mkdir(exportPath, 0755); err != nil {
			log.Printf("[/series/export] failed to export: could not mkdir %s", exportPath)
			w.WriteHeader(400)
			return
		}
		log.Printf("[/series/export] exporting series %s to %s", series.Name, exportPath)
		exporter := ExportSeries(series, ExportOptions{
			Path: exportPath,
			Name: fmt.Sprintf("Export %s", series.Name),
		})
		go func() {
			err := RunJob(exporter)
			if err != nil {
				log.Printf("[/series/export] export job failed: %v", err)
				return
			}
		}()
	})

	http.HandleFunc("/timelines/vectors/export", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		r.ParseForm()
		vectorID := vaas.ParseInt(r.PostForm.Get("vector_id"))
		vector := GetVector(vectorID)
		if vector == nil {
			http.Error(w, "no such vector", 404)
			return
		}
		exportPath := fmt.Sprintf("%s/export-%d-%d/", os.TempDir(), vector.ID, rand.Int63())
		if err := os.Mkdir(exportPath, 0755); err != nil {
			log.Printf("[/series/export] failed to export: could not mkdir %s", exportPath)
			w.WriteHeader(400)
			return
		}
		log.Printf("[/series/export] exporting vector %s to %s", vector.Vector.Pretty(), exportPath)
		exporter := ExportVector(vector.Vector, ExportOptions{
			Path: exportPath,
			Name: fmt.Sprintf("Export %s", vector.Vector.Pretty()),
		})
		go func() {
			err := RunJob(exporter)
			if err != nil {
				log.Printf("[/timelines/vectors/export] export job failed: %v", err)
				return
			}
		}()
	})
}
