package skyhook

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
)

// we assume all videos are constant 25 fps
// TODO: use per-clip or per-video fps
// re-encoding is likely needed to work with skyhook (if video has variable framerate)
// ffmpeg seeking is only fast and frame-accurate with constant framerate
const FPS int = 25

type Video struct {
	ID int
	Name string
	Ext string
	Percent int
}

type Clip struct {
	ID int
	Video Video
	Frames int
	Width int
	Height int
}

type ClipSlice struct {
	Clip Clip
	Start int
	End int
}

func (slice ClipSlice) String() string {
	return fmt.Sprintf("%d[%d:%d]", slice.Clip.ID, slice.Start, slice.End)
}

func (slice ClipSlice) Length() int {
	return slice.End - slice.Start
}

const VideoQuery = "SELECT id, name, ext, percent FROM videos"

func videoListHelper(rows *Rows) []Video {
	videos := []Video{}
	for rows.Next() {
		var video Video
		rows.Scan(&video.ID, &video.Name, &video.Ext, &video.Percent)
		videos = append(videos, video)
	}
	return videos
}

func ListVideos() []Video {
	rows := db.Query(VideoQuery)
	return videoListHelper(rows)
}

func GetVideo(id int) *Video {
	rows := db.Query(VideoQuery + " WHERE id = ?", id)
	videos := videoListHelper(rows)
	if len(videos) == 1 {
		video := videos[0]
		return &video
	} else {
		return nil
	}
}

const ClipQuery = "SELECT c.id, v.id, v.name, v.ext, c.nframes, c.width, c.height FROM clips AS c, videos AS v WHERE v.id = c.video_id"

func clipListHelper(rows *Rows) []Clip {
	var clips []Clip
	for rows.Next() {
		var clip Clip
		rows.Scan(&clip.ID, &clip.Video.ID, &clip.Video.Name, &clip.Video.Ext, &clip.Frames, &clip.Width, &clip.Height)
		clips = append(clips, clip)
	}
	return clips
}

func (video Video) ListClips() []Clip {
	rows := db.Query(ClipQuery + " AND c.video_id = ? ORDER BY c.id", video.ID)
	return clipListHelper(rows)
}

func GetClip(id int) *Clip {
	rows := db.Query(ClipQuery + " AND c.id = ?", id)
	clips := clipListHelper(rows)
	if len(clips) == 1 {
		clip := clips[0]
		return &clip
	} else {
		return nil
	}
}

func (video Video) AddClip(frames int, width int, height int) *Clip {
	res := db.Exec(
		"INSERT INTO clips (video_id, nframes, width, height) VALUES (?, ?, ?, ?)",
		video.ID, frames, width, height,
	)
	return GetClip(res.LastInsertId())
}

func (video Video) Uniform(unit int) ClipSlice {
	clips := video.ListClips()

	// select a clip
	clip := func() Clip {
		if unit == 0 {
			return clips[rand.Intn(len(clips))]
		}
		weights := make([]int, len(clips))
		sum := 0
		for i, clip := range clips {
			weights[i] = (clip.Frames+unit-1) / unit
			if weights[i] < 0 {
				weights[i] = 0
			}
			sum += weights[i]
		}
		r := rand.Intn(sum)
		for i, clip := range clips {
			r -= weights[i]
			if r <= 0 {
				return clip
			}
		}

		// shouldn't happen
		return clips[len(clips)-1]
	}()

	// select frame
	var start, end int
	if unit == 0 {
		start = 0
		end = clip.Frames
	} else {
		idx := rand.Intn((clip.Frames+unit-1)/unit)
		start = idx*unit
		end = (idx+1)*unit
		if end > clip.Frames {
			end = clip.Frames
		}
	}

	return ClipSlice{clip, start, end}
}

func (video Video) Delete() {
	err := os.RemoveAll(fmt.Sprintf("clips/%d", video.ID))
	if err != nil {
		log.Printf("[video] warning: error deleting video %s (id=%d): %v", video.Name, video.ID, err)
	}
	db.Exec("DELETE FROM clips WHERE video_id = ?", video.ID)
	db.Exec("DELETE FROM videos WHERE id = ?", video.ID)
}

func pad6(x int) string {
	s := fmt.Sprintf("%d", x)
	for len(s) < 6 {
		s = "0" + s
	}
	return s
}

func (clip Clip) Fname(index int) string {
	if clip.Video.Ext == "jpeg" {
		return fmt.Sprintf("clips/%d/%d/%s.jpg", clip.Video.ID, clip.ID, pad6(index))
	} else {
		return fmt.Sprintf("clips/%d/%d.%s", clip.Video.ID, clip.ID, clip.Video.Ext)
	}
}

func (clip Clip) ToSlice() ClipSlice {
	return ClipSlice{clip, 0, clip.Frames}
}

func init() {
	http.HandleFunc("/videos", func(w http.ResponseWriter, r *http.Request) {
		JsonResponse(w, ListVideos())
	})

	http.HandleFunc("/videos/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		r.ParseForm()
		videoID, _ := strconv.Atoi(r.PostForm.Get("video_id"))
		video := GetVideo(videoID)
		if video == nil {
			w.WriteHeader(404)
			return
		}
		video.Delete()
	})

	http.HandleFunc("/clips/get", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		id, _ := strconv.Atoi(r.Form.Get("id"))
		start, _ := strconv.Atoi(r.Form.Get("start"))
		end, _ := strconv.Atoi(r.Form.Get("end"))
		contentType := r.Form.Get("type")
		clip := GetClip(id)
		if clip == nil {
			w.WriteHeader(404)
			return
		}
		if end == 0 {
			http.ServeFile(w, r, clip.Fname(0))
			return
		}
		slice := ClipSlice{*clip, start, end}
		rd := ReadVideo(slice, slice.Clip.Width, slice.Clip.Height)
		defer rd.Close()
		if contentType == "jpeg" {
			// return image
			im, err := rd.Read()
			if err != nil {
				log.Printf("[/clips/get] error reading frame (%v): %v", slice, err)
				http.Error(w, "error reading frame", 400)
				return
			}
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write(im.AsJPG())
		} else if contentType == "mp4" {
			vout, cmd := MakeVideo(rd, slice.Clip.Width, slice.Clip.Height)
			_, err := io.Copy(w, vout)
			if err != nil {
				log.Printf("[/clips/get] error reading video (%v): %v", slice, err)
			}
			cmd.Wait()
		}
	})
}
