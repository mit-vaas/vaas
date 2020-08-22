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
	"path/filepath"
	"strings"
)

func ImportFromExport(path string) error {
	log.Printf("[import] attempting import from %s", path)
	bytes, err := ioutil.ReadFile(filepath.Join(path, "meta.json"))
	if err != nil {
		return err
	}
	var meta ExportMetadata
	vaas.JsonUnmarshal(bytes, &meta)

	// create the series
	timeline := NewTimeline(meta.Names[0])
	vector := make([]*DBSeries, len(meta.Names))
	for i := range vector {
		vector[i] = NewSeries(timeline.ID, meta.Names[i], meta.Types[i])
	}

	// create segments while importing the data
	// we create new segment for each exported slice
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return err
	}
	seenLabels := make(map[string]*DBSegment)
	for _, fi := range files {
		if fi.Name() == "meta.json" {
			continue
		}
		fname := filepath.Join(path, fi.Name())
		basename := strings.Split(fi.Name(), ".")[0]
		ext := strings.Split(fi.Name(), ".")[1]
		parts := strings.Split(basename, "_")
		if len(parts) != 4 {
			return fmt.Errorf("filename doesn't follow export format: %s", fi.Name())
		}
		label := strings.Join(parts[0:3], "_")
		start := vaas.ParseInt(parts[1])
		end := vaas.ParseInt(parts[2])
		idx := vaas.ParseInt(parts[3])


		// get segment
		if seenLabels[label] == nil {
			seenLabels[label] = timeline.AddSegment(label, end-start, vaas.FPS)
		}
		segment := seenLabels[label]

		// add item
		format := ext
		if format == "jpg" {
			format = "jpeg"
		}
		dims := [2]int{0, 0}
		if format == "jpeg" {
			im := vaas.ImageFromFile(fname)
			dims = [2]int{im.Width, im.Height}
		}
		item := vector[idx].AddItem(segment.ToSlice(), format, dims, 1)
		os.MkdirAll(filepath.Dir(item.Fname(0)), 0755)

		// copy the file
		err := func() error {
			src, err := os.Open(fname)
			if err != nil {
				return err
			}
			defer src.Close()
			dst, err := os.Create(item.Fname(0))
			if err != nil {
				return err
			}
			defer dst.Close()
			if _, err := io.Copy(dst, src); err != nil {
				return err
			}
			return nil
		}()
		if err != nil {
			return err
		}

		if format == "mp4" {
			// fix video with/height
			ProbeVideo(item)
		}
	}

	return nil
}

// unzip the filename to a temporary directory, then call another function
// afterwards we will clear the temporary directory
func UnzipThen(fname string, f func(path string) error) error {
	tmpDir, err := ioutil.TempDir("", "unzip")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	err = vaas.Command(
		"unzip", vaas.CommandOptions{
			NoStdin: true,
			NoStdout: true,
			OnlyDebug: true,
		},
		"unzip", "-j", "-d", tmpDir, fname,
	).Wait()
	if err != nil {
		return err
	}
	return f(tmpDir)
}

func init() {
	http.HandleFunc("/import/from-export/local", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		path := r.PostForm.Get("path")

		go func() {
			var err error
			if strings.HasSuffix(path, ".zip") {
				err = UnzipThen(path, ImportFromExport)
			} else {
				err = ImportFromExport(path)
			}
			log.Printf("[import-from-export] completed on %s: %v", path, err)
		}()
	})

	http.HandleFunc("/import/from-export/upload", func(w http.ResponseWriter, r *http.Request) {
		// expect a zip file containing a Vaas export
		log.Printf("[/import/from-export/upload] handling import from upload request")
		HandleUpload(w, r, func(fname string) error {
			log.Printf("[/import/from-export/upload] importing from upload request: %s", fname)

			// move the file so it won't get cleaned up by HandleUpload
			newFname := filepath.Join(os.TempDir(), fmt.Sprintf("%d%s", rand.Int63(), filepath.Ext(fname)))
			if err := os.Rename(fname, newFname); err != nil {
				return err
			}

			go func() {
				err := UnzipThen(newFname, ImportFromExport)
				if err != nil {
					log.Printf("[/import/from-export/upload] error: %v", err)
				}
			}()
			return nil
		})
	})
}
