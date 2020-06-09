package skyhook

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func progressUpdater(video Video, initialPercent int, targetPercent int) func(frac float64) {
	var lastUpdate time.Time
	return func(frac float64) {
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
		log.Printf("[video_import (%s)] update progress %d", video.Name, percent)
		db.Exec("UPDATE videos SET percent = ? WHERE id = ?", percent, video.ID)
	}
}

func Transcode(src string, dst string, video Video, initialPercent int) error {
	log.Printf("[video_import (%s)] transcode [%s] -> [%s]", video.Name, src, dst)

	opts := CommandOptions{
		NoStdin: true,
		Stderr: new(io.ReadCloser),
	}
	cmd, _, _ := Command(
		"ffmpeg-transcode", opts,
		"ffmpeg",
		"-progress", "pipe:2",
		"-i", src,
		"-vcodec", "libx264", "-vf", fmt.Sprintf("fps=%v", FPS),
		"-an",
		"-f", "mp4",
		dst,
	)
	stderr := *opts.Stderr

	updateProgress := progressUpdater(video, initialPercent, 100)

	rd := bufio.NewReader(stderr)
	var duration int
	for {
		line, err := rd.ReadString('\n')
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Duration:") {
			line = strings.Split(line, "Duration: ")[1]
			line = strings.Split(line, ", ")[0]
			parts := strings.Split(line, ":")
			h, _ := strconv.ParseFloat(parts[0], 64)
			m, _ := strconv.ParseFloat(parts[1], 64)
			s, _ := strconv.ParseFloat(parts[2], 64)
			duration = int(h*3600*1000 + m*60*1000 + s*1000)
		} else if duration > 0 && strings.HasPrefix(line, "out_time_ms=") {
			line = strings.Split(line, "=")[1]
			elapsed, _ := strconv.ParseFloat(line, 64)
			frac := elapsed/float64(duration)
			updateProgress(frac)
		}
	}

	cmd.Wait()
	db.Exec("UPDATE videos SET percent = 100 WHERE id = ?", video.ID)
	return nil
}

// run ffprobe on a clip and fix it's frames, width, height
func ProbeClip(clip *Clip) {
	width, height, duration, err := Ffprobe(clip.Fname(0))
	if err != nil {
		log.Printf("[video_import] probe failed: %v", err)
		return
	}
	nframes := int(duration * float64(FPS))
	db.Exec("UPDATE clips SET nframes = ?, width = ?, height = ? WHERE id = ?", nframes, width, height, clip.ID)
}

func ImportLocal(fname string) func(video Video) error {
	return func(video Video) error {
		// we will fix the frames/width/height later
		clip := video.AddClip(1, 1920, 1080)
		err := Transcode(fname, clip.Fname(0), video, 0)
		if err != nil {
			return err
		}
		ProbeClip(clip)
		return nil
	}
}

func ImportYoutube(url string) func(video Video) error {
	return func(video Video) error {
		clip := video.AddClip(1, 1920, 1080)

		// download the video
		log.Printf("[video_import (%s)] youtube: download video from %s", video.Name, url)
		tmpFname := fmt.Sprintf("%s/%d.mp4", os.TempDir(), rand.Int63())
		defer os.Remove(tmpFname)
		cmd, _, stdout := Command(
			"youtube-dl", CommandOptions{NoStdin: true},
			"youtube-dl",
			"-o", tmpFname, "--newline",
			url,
		)
		updateProgress := progressUpdater(video, 0, 50)
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
			line = strings.Split(line, "%")[0]
			line = strings.Split(line, "[download] ")[1]
			percent, _ := strconv.ParseFloat(line, 64)
			updateProgress(percent/100)
		}

		cmd.Wait()
		log.Printf("[video_import (%s)] youtube: download complete, begin transcode", video.Name)
		err := Transcode(tmpFname, clip.Fname(0), video, 50)
		if err != nil {
			return err
		}
		ProbeClip(clip)
		return nil
	}
}

func ImportVideo(name string, f func(Video) error) {
	res := db.Exec("INSERT INTO videos (name, ext, percent) VALUES (?, 'mp4', 0)", name)
	video := GetVideo(res.LastInsertId())
	os.Mkdir(fmt.Sprintf("clips/%d", video.ID), 0755)
	log.Printf("[video_import (%s)] import id=%d", name, video.ID)
	err := f(*video)
	if err != nil {
		log.Printf("[video_import (%s)] import error: %v", err)
		return
	}
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
