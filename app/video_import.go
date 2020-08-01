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
				return nil, err
			}
		} else if symlink {
			err := os.Symlink(fname, item.Fname(0))
			if err != nil {
				return nil, err
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
				return nil, err
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
				return nil, fmt.Errorf("youtube-dl error: %v", err)
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
			return nil, err
		}
		log.Printf("[video_import (%s)] youtube: download complete, begin transcode", segment.Name)
		err := Transcode(tmpFname, item, 50)
		if err != nil {
			return nil, err
		}
		ProbeVideo(item)
		return item, nil
	}
}

func ImportVideo(r *http.Request, f func(DBSeries, *DBSegment) (*DBItem, error)) (*DBItem, error) {
	seriesID := vaas.ParseInt(r.PostForm.Get("series_id"))
	series := GetSeries(seriesID)
	if series == nil {
		return nil, fmt.Errorf("no such series")
	}
	var segment *DBSegment
	if r.PostForm["segment_id"] != nil && r.PostForm.Get("segment_id") != "" {
		segmentID := vaas.ParseInt(r.PostForm.Get("segment_id"))
		segment = GetSegment(segmentID)
		if segment == nil {
			return nil, fmt.Errorf("no such segment")
		}
	}
	os.Mkdir(fmt.Sprintf("items/%d", series.ID), 0755)
	item, err := f(*series, segment)
	if err != nil {
		log.Printf("[video_import] import error: %v", err)
		series.Delete()
		return nil, err
	}
	db.Exec("UPDATE items SET percent = 100 WHERE id = ?", item.ID)
	return item, nil
}

func init() {
	http.HandleFunc("/import/local", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		path := r.PostForm.Get("path")
		symlink := r.PostForm.Get("symlink") == "yes"
		transcode := r.PostForm.Get("transcode") == "yes"

		// if it's a directory, we need to list all the files
		info, err := os.Stat(path)
		if err != nil {
			http.Error(w, fmt.Sprintf("%s does not exist", path), 400)
			return
		}
		if !info.IsDir() {
			go ImportVideo(r, ImportLocal(path, symlink, transcode))
			return
		}
		files, err := ioutil.ReadDir(path)
		if err != nil {
			http.Error(w, fmt.Sprintf("error listing files: %v", err), 400)
			return
		}
		go func() {
			for _, fi := range files {
				fname := filepath.Join(path, fi.Name())
				ImportVideo(r, ImportLocal(fname, symlink, transcode))
			}
		}()
	})

	http.HandleFunc("/import/youtube", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		url := r.PostForm.Get("url")
		go ImportVideo(r, ImportYoutube(url))
	})
}
