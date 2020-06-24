package skyhook

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
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
}

type Exporter struct {
	refs [][]DataRef
	opts ExportOptions

	l []string
	mu sync.Mutex
}

func NewExporter(refs [][]DataRef, opts ExportOptions) *Exporter {
	return &Exporter{refs: refs, opts: opts}
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

func (e *Exporter) Run(statusFunc func(string)) error {
	statusFunc("Exporting")

	exportVideo := func(slice Slice, ref ConcreteRef, prefix string) error {
		if ref.Item == nil {
			return fmt.Errorf("error exporting video: video must be from item not encoded")
		}
		rd := ReadVideo(*ref.Item, slice, ReadVideoOptions{})
		defer rd.Close()
		if slice.Length() == 1 {
			im, err := rd.Read()
			if err != nil {
				return err
			}
			if err := ioutil.WriteFile(prefix + ".jpg", im.AsJPG(), 0644); err != nil {
				return err
			}
		} else {
			file, err := os.Create(prefix + ".mp4")
			if err != nil {
				return err
			}
			defer file.Close()
			stdout, cmd := MakeVideo(rd, ref.Item.Width, ref.Item.Height)
			io.Copy(file, stdout)
			cmd.Wait()
		}
		return nil
	}

	exportOther := func(slice Slice, ref ConcreteRef, prefix string) error {
		var data Data
		if ref.Item != nil {
			buf := ref.Item.Load(slice)
			var err error
			data, err = buf.Reader().Read(slice.Length())
			if err != nil {
				return fmt.Errorf("error reading buffer: %v", err)
			}
		} else {
			data = ref.Data.Slice(slice.Start - ref.Slice.Start, slice.End - ref.Slice.End)
		}
		fname := prefix + ".json"
		if err := ioutil.WriteFile(fname, data.Encode(), 0644); err != nil {
			return fmt.Errorf("error writing json to %s: %v", fname, err)
		}
		return nil
	}

	fmt.Println("start", e.refs)
	err := EnumerateDataRefs(e.refs, func(slice Slice, refs []ConcreteRef) error {
		fmt.Println("one", refs)
		prefix := fmt.Sprintf("%s/%d_%d_%d", e.opts.Path, slice.Segment.ID, slice.Start, slice.End)
		for i, ref := range refs {
			var err error
			curPrefix := prefix + fmt.Sprintf("_%d", i)
			if ref.Type() == VideoType {
				err = exportVideo(slice, ref, curPrefix)
			} else {
				err = exportOther(slice, ref, curPrefix)
			}
			if err != nil {
				return err
			}
		}
		e.mu.Lock()
		e.l = append(e.l, prefix)
		e.mu.Unlock()
		return nil
	})
	return err
}

func (e *Exporter) Detail() interface{} {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.l
}
