package skyhook

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func progressUpdater(series Series, initialPercent int, targetPercent int) func(frac float64, msg string) {
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
		log.Printf("[video_import (%s)] update progress %d (%s)", series.Name, percent, msg)
		db.Exec("UPDATE series SET percent = ? WHERE id = ?", percent, series.ID)
	}
}

func Transcode(src string, dst string, series Series, initialPercent int) error {
	log.Printf("[video_import (%s)] transcode [%s] -> [%s]", series.Name, src, dst)

	opts := CommandOptions{
		NoStdin: true,
		NoStdout: true,
		NoPrintStderr: true,
	}
	cmd := Command(
		"ffmpeg-transcode", opts,
		"ffmpeg",
		"-progress", "pipe:2",
		"-i", src,
		"-vcodec", "libx264", "-vf", fmt.Sprintf("fps=%v", FPS),
		"-an",
		"-f", "mp4",
		dst,
	)
	stderr := cmd.Stderr()

	updateProgress := progressUpdater(series, initialPercent, 100)

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
			duration = parseFfmpegTime(str)
			log.Printf("[ffmpeg-transcode] detect duration [%d] (%s)", duration, line)
		} else if duration > 0 && strings.HasPrefix(line, "out_time=") {
			str := strings.Split(line, "=")[1]
			elapsed := parseFfmpegTime(str)
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
func ProbeVideo(item *Item) {
	width, height, duration, err := Ffprobe(item.Fname(0))
	if err != nil {
		log.Printf("[video_import] probe failed: %v", err)
		return
	}
	frames := int(duration * float64(FPS))
	db.Exec("UPDATE items SET start = 0, end = ?, width = ?, height = ? WHERE id = ?", frames, width, height, item.ID)
	db.Exec("UPDATE segments SET frames = ? WHERE id = ?", frames, item.Slice.Segment.ID)
}

func ImportLocal(fname string) func(series Series) error {
	return func(series Series) error {
		// we will fix the frames/width/height later
		segment := series.Timeline.AddSegment(filepath.Base(fname), 1, FPS)
		item := series.AddItem(segment.ToSlice(), "mp4", [2]int{1920, 1080})
		err := Transcode(fname, item.Fname(0), series, 0)
		if err != nil {
			return err
		}
		ProbeVideo(item)
		return nil
	}
}

func ImportYoutube(url string) func(series Series) error {
	return func(series Series) error {
		segment := series.Timeline.AddSegment(url, 1, FPS)
		item := series.AddItem(segment.ToSlice(), "mp4", [2]int{1920, 1080})

		// download the video
		log.Printf("[video_import (%s)] youtube: download video from %s", series.Name, url)
		tmpFname := fmt.Sprintf("%s/%d.mp4", os.TempDir(), rand.Int63())
		defer os.Remove(tmpFname)
		cmd := Command(
			"youtube-dl", CommandOptions{NoStdin: true},
			"youtube-dl",
			"-o", tmpFname, "--newline",
			url,
		)
		stdout := cmd.Stdout()
		updateProgress := progressUpdater(series, 0, 50)
		rd := bufio.NewReader(stdout)
		for {
			line, err := rd.ReadString('\n')
			if err == io.EOF {
				break
			} else if err != nil {
				return fmt.Errorf("youtube-dl error: %v", err)
			}
			if !strings.HasPrefix(line, "[download] ") || !strings.Contains(line, "% of") {
				continue
			}
			str := strings.Split(line, "%")[0]
			str = strings.Split(str, "[download] ")[1]
			str = strings.TrimSpace(str)
			percent := ParseFloat(str)
			updateProgress(percent/100, line)
		}

		if err := cmd.Wait(); err != nil {
			log.Printf("[video_import (%s)] youtube: download failed", series.Name)
			return err
		}
		log.Printf("[video_import (%s)] youtube: download complete, begin transcode", series.Name)
		err := Transcode(tmpFname, item.Fname(0), series, 50)
		if err != nil {
			return err
		}
		ProbeVideo(item)
		return nil
	}
}

func ImportVideo(name string, f func(Series) error) {
	res := db.Exec("INSERT INTO timelines (name) VALUES (?)", name)
	timeline := GetTimeline(res.LastInsertId())
	res = db.Exec("INSERT INTO series (timeline_id, name, type, data_type, percent) VALUES (?, ?, 'data', 'video', 0)", timeline.ID, name)
	series := GetSeries(res.LastInsertId())
	os.Mkdir(fmt.Sprintf("items/%d", series.ID), 0755)
	log.Printf("[video_import (%s)] import id=%d", name, series.ID)
	err := f(*series)
	if err != nil {
		log.Printf("[video_import (%s)] import error: %v", series.Name, err)
		series.Delete()
		return
	}
	db.Exec("UPDATE series SET percent = 100 WHERE id = ?", series.ID)
}

func init() {
	http.HandleFunc("/import/local", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		name := r.PostForm.Get("name")
		path := r.PostForm.Get("path")
		go ImportVideo(name, ImportLocal(path))
	})

	http.HandleFunc("/import/youtube", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		name := r.PostForm.Get("name")
		url := r.PostForm.Get("url")
		go ImportVideo(name, ImportYoutube(url))
	})
}
