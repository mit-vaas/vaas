package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
)

type LabelSet struct {
	ID int
	Name string
	SelFrames int
	SrcVideo Video
	Video Video
	Type DataType
}

const LabelSetQuery = `
	SELECT ls.id, ls.name, ls.sel_frames, ls.label_type,
		sv.id, sv.name, sv.ext,
		v.id, v.name, v.ext
	FROM label_sets AS ls, videos AS sv, videos AS v
	WHERE sv.id = ls.src_video AND v.id = ls.video_id
`

func labelSetListHelper(rows Rows) []LabelSet {
	var sets []LabelSet
	for rows.Next() {
		var ls LabelSet
		rows.Scan(
			&ls.ID, &ls.Name, &ls.SelFrames, &ls.Type,
			&ls.SrcVideo.ID, &ls.SrcVideo.Name, &ls.SrcVideo.Ext,
			&ls.Video.ID, &ls.Video.Name, &ls.Video.Ext,
		)
		sets = append(sets, ls)
	}
	return sets
}

func ListLabelSets() []LabelSet {
	rows := db.Query(LabelSetQuery)
	return labelSetListHelper(rows)
}

func GetLabelSet(id int) *LabelSet {
	rows := db.Query(LabelSetQuery + " AND ls.id = ?", id)
	sets := labelSetListHelper(rows)
	if len(sets) == 1 {
		ls := sets[0]
		return &ls
	} else {
		return nil
	}
}

func NewLabelSet(name string, srcVideoID int, labelType DataType) (*LabelSet, error) {
	if labelType != DetectionType && labelType != TrackType && labelType != ClassType {
		return nil, fmt.Errorf("invalid label type %s", labelType)
	}
	res := db.Exec("INSERT INTO videos (name, ext) VALUES (?, 'jpeg')", "labels: " + name)
	videoID := res.LastInsertId()
	res = db.Exec(
		"INSERT INTO label_sets (name, sel_frames, src_video, video_id, label_type) VALUES (?, ?, ?, ?, ?)",
		name, 1, srcVideoID, videoID, labelType,
	)
	lsID := res.LastInsertId()
	ls := GetLabelSet(lsID)
	log.Printf("[annotate] created new label set %d", ls.ID)
	return ls, nil
}

func (ls LabelSet) Next() []Image {
	return ls.SrcVideo.Uniform(ls.SelFrames)
}

type LabeledClip struct {
	Slice ClipSlice
	Type DataType
	Label interface{}
}

func (clipLabel *LabeledClip) GetSlice(start int, end int) *LabeledClip {
	prevSlice := clipLabel.Slice
	slice := ClipSlice{prevSlice.Clip, prevSlice.Start+start, prevSlice.Start+end}
	var label interface{}
	if clipLabel.Type == DetectionType || clipLabel.Type == TrackType {
		detections := clipLabel.Label.([][]Detection)
		label = detections[start:end+1]
	}
	return &LabeledClip{
		Slice: slice,
		Type: clipLabel.Type,
		Label: label,
	}
}

func (ls LabelSet) Get(index int) *LabeledClip {
	clips := ls.Video.ListClips()
	if index < 0 || index >= len(clips) {
		return nil
	}
	clip := clips[index]
	return ls.GetByClip(clip)
}

func (ls LabelSet) Length() int {
	// TODO: what we really need is a function that checks which labels
	//       are available on the filesystem
	return len(ls.Video.ListClips())
}

func (ls LabelSet) GetByClip(clip Clip) *LabeledClip {
	if clip.Video.ID != ls.Video.ID {
		return nil
	}
	bytes, err := ioutil.ReadFile(fmt.Sprintf("labels/%d/%d.json", ls.ID, clip.ID))
	if err != nil {
		return &LabeledClip{clip.ToSlice(), ls.Type, nil}
	}
	if ls.Type == DetectionType || ls.Type == TrackType {
		/*if clip.Frames == 1 {
			var label [][][2]int
			if err := json.Unmarshal(bytes, &label); err != nil {
				panic(err)
			}
			return &LabeledClip{clip, ls.Type, label}
		} else {
			var label [][][][2]int
			if err := json.Unmarshal(bytes, &label); err != nil {
				panic(err)
			}
			return &LabeledClip{clip, ls.Type, label}
		}*/
		var label [][]Detection
		if err := json.Unmarshal(bytes, &label); err != nil {
			panic(err)
		}
		return &LabeledClip{clip.ToSlice(), ls.Type, label}
	}
	return nil
}

func (ls LabelSet) AddClip(images []Image, label interface{}) {
	clip := ls.Video.AddClip(len(images), images[0].Width, images[0].Height)
	os.Mkdir(fmt.Sprintf("clips/%d", ls.Video.ID), 0755)
	os.Mkdir(fmt.Sprintf("labels/%d", ls.Video.ID), 0755)
	if ls.Video.Ext == "jpeg" {
		os.Mkdir(fmt.Sprintf("clips/%d/%d", ls.Video.ID, clip.ID), 0755)
		for i, im := range images {
			bytes := im.AsJPG()
			if err := ioutil.WriteFile(clip.Fname(i), bytes, 0644); err != nil {
				panic(err)
			}
		}
	}

	ls.UpdateLabel(*clip, label)
}

func (ls LabelSet) UpdateLabel(clip Clip, label interface{}) {
	bytes, err := json.Marshal(label)
	if err != nil {
		panic(err)
	}
	fname := fmt.Sprintf("labels/%d/%d.json", ls.Video.ID, clip.ID)
	if err := ioutil.WriteFile(fname, bytes, 0644); err != nil {
		panic(err)
	}
}

type LabelsResponse struct {
	URL string
	Width int
	Height int
	Index int
	UUID string
	Labels interface{}
}

type DetectionLabelRequest struct {
	ID int `json:"id"`
	Index int `json:"index"`
	UUID string `json:"uuid"`
	Labels [][][2]int `json:"labels"`
}

//const VisualizeMaxFrames int = 30*30
const VisualizeMaxFrames int = 5*30

type VisualizeResponse struct {
	PreviewURL string
	URL string
	Width int
	Height int
	UUID string
}

func init() {
	http.HandleFunc("/labelsets", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			jsonResponse(w, ListLabelSets())
			return
		} else if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		r.ParseForm()
		name := r.PostForm.Get("name")
		labelType := r.PostForm.Get("type")
		srcID, _ := strconv.Atoi(r.PostForm.Get("src"))
		ls, err := NewLabelSet(name, srcID, DataType(labelType))
		if err != nil {
			w.WriteHeader(400)
			return
		}
		jsonResponse(w, ls)
	})

	http.HandleFunc("/labelsets/labels", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		lsID, _ := strconv.Atoi(r.Form.Get("id"))
		index, _ := strconv.Atoi(r.Form.Get("index"))
		ls := GetLabelSet(lsID)
		if ls == nil {
			log.Printf("[annotate] no labelset with id %d", lsID)
			w.WriteHeader(400)
			return
		}

		clipLabel := ls.Get(index)
		if clipLabel != nil {
			fmt.Println(clipLabel)
			jsonResponse(w, LabelsResponse{
				URL: fmt.Sprintf("/clips/get?id=%d", clipLabel.Slice.Clip.ID),
				Width: clipLabel.Slice.Clip.Width,
				Height: clipLabel.Slice.Clip.Height,
				Index: index,
				Labels: clipLabel.Label,
			})
			return
		}

		images := ls.Next()
		uuid := cache.Add(images)
		jsonResponse(w, LabelsResponse{
			URL: fmt.Sprintf("/cache/preview?id=%s&type=jpeg", uuid),
			Width: images[0].Width,
			Height: images[0].Height,
			Index: -1,
			UUID: uuid,
		})
	})

	http.HandleFunc("/labelsets/detection-label", func(w http.ResponseWriter, r *http.Request) {
		var request DetectionLabelRequest
		if err := jsonRequest(w, r, &request); err != nil {
			return
		}

		ls := GetLabelSet(request.ID)
		if ls == nil {
			log.Printf("[annotate] no labelset with id %d", request.ID)
			w.WriteHeader(400)
			return
		}

		if request.Index == -1 {
			item := cache.Get(request.UUID)
			if item == nil {
				log.Printf("[annotate] detection-label: no item for index=%d, uuid=%s", request.Index, request.UUID)
				w.WriteHeader(400)
				return
			}
			images := item.([]Image)
			log.Printf("[annotate] add labels for new clip to ls %d", ls.ID)
			ls.AddClip(images, request.Labels)
			w.WriteHeader(200)
			return
		}

		clipLabel := ls.Get(request.Index)
		if clipLabel == nil {
			log.Printf("[annotate] detection-label: no clip at index %d", request.Index)
			w.WriteHeader(400)
			return
		}
		log.Printf("[annotate] update labels for clip %d in ls %d", clipLabel.Slice.Clip.ID, ls.ID)
		ls.UpdateLabel(clipLabel.Slice.Clip, request.Labels)
	})

	http.HandleFunc("/labelsets/visualize", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		lsID, _ := strconv.Atoi(r.Form.Get("id"))
		ls := GetLabelSet(lsID)
		if ls == nil {
			log.Printf("[annotate] visualize: no label set %d", lsID)
			w.WriteHeader(404)
			return
		}
		// TODO: we should return some kind of data that client can use to access
		//       additional clips via visualize-more; currently they will only be
		//       able to see the first 30 sec in the first clip
		log.Printf("[annotate] visualize: loading labeled clip %d/%d", lsID, 0)
		clipLabel := ls.Get(0)
		if clipLabel == nil {
			log.Printf("[annotate] visualize: no clips in label set %d", lsID)
			w.WriteHeader(400)
			return
		}
		end := clipLabel.Slice.Clip.Frames-1
		if end >= VisualizeMaxFrames {
			end = VisualizeMaxFrames-1
		}
		log.Printf("[annotate] visualize: loading preview for clip %d (frames %d to %d)", clipLabel.Slice.Clip.ID, 0, end)
		pc := clipLabel.GetSlice(0, end).LoadPreview()
		uuid := cache.Add(pc)
		log.Printf("[annotate] visualize: cached preview with %d frames, uuid=%s", len(pc.Images), uuid)
		jsonResponse(w, VisualizeResponse{
			PreviewURL: fmt.Sprintf("/cache/preview?id=%s&type=jpeg", uuid),
			URL: fmt.Sprintf("/cache/view?id=%s&type=mp4", uuid),
			Width: clipLabel.Slice.Clip.Width,
			Height: clipLabel.Slice.Clip.Height,
			UUID: uuid,
		})
	})

	/*http.HandleFunc("/labelsets/visualize-more", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		lsID, _ := strconv.Atoi(r.Form.Get("id"))
		clipID, _ := strconv.Atoi(r.Form.Get("clip_id"))
		offset, _ := strconv.Atoi(r.Form.Get("offset"))
	})*/
}
