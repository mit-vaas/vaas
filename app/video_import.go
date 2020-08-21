package app

import (
	"../vaas"

	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func progressUpdater(item *DBItem, initialPercent int, targetPercent int) func(frac float64, msg string) {
	var lastUpdate time.Time
	return func(frac float64, msg string) {
		if time.Now().Sub(lastUpdate) < 2*time.Second {
			return
		}
		lastUpdate = time.Now()
		percent := initialPercent + int(frac*float64(targetPercent-initialPercent))
		if percent < initialPercent {
			percent = initialPercent
		} else if percent > targetPercent {
			percent = targetPercent-1
		}
		log.Printf("[video_import (%s)] update progress %d (%s)", item.Slice.Segment.Name, percent, msg)
		db.Exec("UPDATE items SET percent = ? WHERE id = ?", percent, item.ID)
	}
}

func Transcode(src string, item *DBItem, initialPercent int) error {
	dst := item.Fname(0)
	log.Printf("[video_import (%s)] transcode [%s] -> [%s]", item.Slice.Segment.Name, src, dst)

	opts := vaas.CommandOptions{
		NoStdin: true,
		NoStdout: true,
		NoPrintStderr: true,
	}
	cmd := vaas.Command(
		"ffmpeg-transcode", opts,
		"ffmpeg",
		"-threads", "2",
		"-progress", "pipe:2",
		"-i", src,
		"-vcodec", "libx264", "-vf", fmt.Sprintf("fps=%v", vaas.FPS),
		"-an",
		"-f", "mp4",
		dst,
	)
	stderr := cmd.Stderr()

	updateProgress := progressUpdater(item, initialPercent, 100)

	rd := bufio.NewReader(stderr)
	var duration int
	var lastLine string
	for {
		line, err := rd.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			lastLine = line
		}

		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		if strings.HasPrefix(line, "Duration:") {
			str := strings.Split(line, "Duration: ")[1]
			str = strings.Split(str, ", ")[0]
			duration = vaas.ParseFfmpegTime(str)
			log.Printf("[ffmpeg-transcode] detect duration [%d] (%s)", duration, line)
		} else if duration > 0 && strings.HasPrefix(line, "out_time=") {
			str := strings.Split(line, "=")[1]
			elapsed := vaas.ParseFfmpegTime(str)
			frac := float64(elapsed)/float64(duration)
			updateProgress(frac, line)
		}
	}

	err := cmd.Wait()
	if err != nil {
		return fmt.Errorf("ffmpeg: %v (last line: %s)", err, lastLine)
	}
	return nil
}

// run ffprobe on a video and fix it's frames, width, height
func ProbeVideo(item *DBItem) {
	width, height, duration, err := vaas.Ffprobe(item.Fname(0))
	if err != nil {
		log.Printf("[video_import] probe failed: %v", err)
		return
	}
	frames := int(duration * float64(vaas.FPS))
	db.Exec("UPDATE items SET start = 0, end = ?, width = ?, height = ? WHERE id = ?", frames, width, height, item.ID)
	db.Exec("UPDATE segments SET frames = ? WHERE id = ?", frames, item.Slice.Segment.ID)
}

func ImportLocal(fname string, symlink bool, transcode bool) func(series DBSeries, segment *DBSegment) (*DBItem, error) {
	return func(series DBSeries, segment *DBSegment) (*DBItem, error) {
		// we will fix the frames/width/height later
		if segment == nil {
			segment = DBTimeline{Timeline: series.Timeline}.AddSegment(filepath.Base(fname), 1, vaas.FPS)
		}
		item := series.AddItem(segment.ToSlice(), "mp4", [2]int{1920, 1080}, 1)
		if transcode {
			err := Transcode(fname, item, 0)
			if err != nil {
				return item, err
			}
		} else if symlink {
			err := os.Symlink(fname, item.Fname(0))
			if err != nil {
				return item, err
			}
		} else {
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
				return item, err
			}
		}
		ProbeVideo(item)
		return item, nil
	}
}

func ImportYoutube(url string) func(series DBSeries, segment *DBSegment) (*DBItem, error) {
	return func(series DBSeries, segment *DBSegment) (*DBItem, error) {
		if segment == nil {
			segment = DBTimeline{Timeline: series.Timeline}.AddSegment(url, 1, vaas.FPS)
		}
		item := series.AddItem(segment.ToSlice(), "mp4", [2]int{1920, 1080}, 1)

		// download the video
		log.Printf("[video_import (%s)] youtube: download video from %s", segment.Name, url)
		tmpFname := fmt.Sprintf("%s/%d.mp4", os.TempDir(), rand.Int63())
		defer os.Remove(tmpFname)
		cmd := vaas.Command(
			"youtube-dl", vaas.CommandOptions{NoStdin: true},
			"youtube-dl",
			"-o", tmpFname, "--newline",
			url,
		)
		stdout := cmd.Stdout()
		updateProgress := progressUpdater(item, 0, 50)
		rd := bufio.NewReader(stdout)
		for {
			line, err := rd.ReadString('\n')
			if err == io.EOF {
				break
			} else if err != nil {
				return item, fmt.Errorf("youtube-dl error: %v", err)
			}
			if !strings.HasPrefix(line, "[download] ") || !strings.Contains(line, "% of") {
				continue
			}
			str := strings.Split(line, "%")[0]
			str = strings.Split(str, "[download] ")[1]
			str = strings.TrimSpace(str)
			percent := vaas.ParseFloat(str)
			updateProgress(percent/100, line)
		}

		if err := cmd.Wait(); err != nil {
			log.Printf("[video_import (%s)] youtube: download failed", segment.Name)
			return item, err
		}
		log.Printf("[video_import (%s)] youtube: download complete, begin transcode", segment.Name)
		err := Transcode(tmpFname, item, 50)
		if err != nil {
			return item, err
		}
		ProbeVideo(item)
		return item, nil
	}
}

func ImportVideo(seriesID int, segmentID *int, f func(DBSeries, *DBSegment) (*DBItem, error)) (*DBItem, error) {
	series := GetSeries(seriesID)
	if series == nil {
		return nil, fmt.Errorf("no such series")
	}
	var segment *DBSegment
	if segmentID != nil {
		segment = GetSegment(*segmentID)
		if segment == nil {
			return nil, fmt.Errorf("no such segment")
		}
	}
	os.Mkdir(fmt.Sprintf("items/%d", series.ID), 0755)
	item, err := f(*series, segment)
	if err != nil {
		log.Printf("[video_import] import error: %v", err)
		if item != nil {
			item.Delete()
		}
		return nil, err
	}
	db.Exec("UPDATE items SET percent = 100 WHERE id = ?", item.ID)
	return item, nil
}

// import either a video or directory of videos
// the import operation itself is done asynchronously
func ImportVideos(seriesID int, segmentID *int, path string, symlink bool, transcode bool) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s does not exist", path)
	}
	if !info.IsDir() {
		go ImportVideo(seriesID, segmentID, ImportLocal(path, symlink, transcode))
		return nil
	}
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return fmt.Errorf("error listing files: %v", err)
	}
	go func() {
		for _, fi := range files {
			fname := filepath.Join(path, fi.Name())
			ImportVideo(seriesID, segmentID, ImportLocal(fname, symlink, transcode))
		}
	}()
	return nil
}

// handle parts of standard upload where we save to a temporary file with same
// extension as uploaded file
func HandleUpload(w http.ResponseWriter, r *http.Request, f func(fname string) error) {
	err := func() error {
		file, fh, err := r.FormFile("file")
		if err != nil {
			return fmt.Errorf("error processing upload: %v", err)
		}
		// write file to a temporary file on disk with same extension
		ext := filepath.Ext(fh.Filename)
		tmpfile, err := ioutil.TempFile("", fmt.Sprintf("*%s", ext))
		if err != nil {
			return fmt.Errorf("error processing upload: %v", err)
		}
		defer os.Remove(tmpfile.Name())
		if _, err := io.Copy(tmpfile, file); err != nil {
			return fmt.Errorf("error processing upload: %v", err)
		}
		if err := tmpfile.Close(); err != nil {
			return fmt.Errorf("error processing upload: %v", err)
		}
		return f(tmpfile.Name())
	}()
	if err != nil {
		log.Printf("[upload %s] error: %v", r.URL.Path, err)
		http.Error(w, err.Error(), 400)
	}
}

func init() {
	http.HandleFunc("/import/local", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "404", 404)
			return
		}
		r.ParseForm()
		path := r.PostForm.Get("path")
		symlink := r.PostForm.Get("symlink") == "yes"
		transcode := r.PostForm.Get("transcode") == "yes"

		seriesID := vaas.ParseInt(r.PostForm.Get("series_id"))
		var segmentID *int
		if r.PostForm["segment_id"] != nil && r.PostForm.Get("segment_id") != "" {
			segmentID = new(int)
			*segmentID = vaas.ParseInt(r.PostForm.Get("segment_id"))
		}

		err := ImportVideos(seriesID, segmentID, path, symlink, transcode)
		if err != nil {
			http.Error(w, err.Error(), 400)
		}
	})

	http.HandleFunc("/import/youtube", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		url := r.PostForm.Get("url")

		seriesID := vaas.ParseInt(r.PostForm.Get("series_id"))
		var segmentID *int
		if r.PostForm["segment_id"] != nil && r.PostForm.Get("segment_id") != "" {
			segmentID = new(int)
			*segmentID = vaas.ParseInt(r.PostForm.Get("segment_id"))
		}

		go ImportVideo(seriesID, segmentID, ImportYoutube(url))
	})

	http.HandleFunc("/import/upload", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		seriesID := vaas.ParseInt(r.Form.Get("series_id"))
		log.Printf("[/import/upload] handling import from upload request, series %d", seriesID)
		HandleUpload(w, r, func(fname string) error {
			log.Printf("[/import/upload] importing from upload request: %s", fname)

			// move the file so it won't get cleaned up by HandleUpload
			// need to do this since ImportVideos processes the file asynchronously
			newFname := filepath.Join(os.TempDir(), fmt.Sprintf("%d%s", rand.Int63(), filepath.Ext(fname)))
			if err := os.Rename(fname, newFname); err != nil {
				return err
			}

			if filepath.Ext(newFname) == ".zip" {
				return UnzipThen(newFname, func(path string) error {
					return ImportVideos(seriesID, nil, path, false, false)
				})
			} else {
				return ImportVideos(seriesID, nil, newFname, false, false)
			}
		})
	})
}
