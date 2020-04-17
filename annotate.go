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
	Unit int
	SrcVideo Video
	Video Video
	Type DataType
}

type Label struct {
	ID int
	LabelSet LabelSet
	Slice ClipSlice
}

const LabelSetQuery = `
	SELECT ls.id, ls.name, ls.unit, ls.label_type,
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
			&ls.ID, &ls.Name, &ls.Unit, &ls.Type,
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
	if labelType != DetectionType && labelType != TrackType && labelType != ClassType && labelType != VideoType {
		return nil, fmt.Errorf("invalid label type %s", labelType)
	}
	var lsID int
	if labelType != VideoType {
		res := db.Exec("INSERT INTO videos (name, ext) VALUES (?, 'jpeg')", "labels: " + name)
		videoID := videoRes.LastInsertId()
		res = db.Exec(
			"INSERT INTO label_sets (name, unit, src_video, video_id, label_type) VALUES (?, ?, ?, ?, ?)",
			name, 1, srcVideoID, videoID, labelType,
		)
		lsID = res.LastInsertId()
	} else {
		// don't have a new video anymore since this label set will just be used for visualization
		// we do set dst_video since that is what will be read for visualizing
		res := db.Exec(
			"INSERT INTO label_sets (name, unit, src_video, video_id, label_type) VALUES (?, ?, ?, ?, ?)",
			name, 1, srcVideoID, srcVideoID, labelType,
		)
		lsID = res.LastInsertId()

		// this is going to be used for visualization, not for annotation
		// we actually pre-populate the annotations by creating labels that point back to the
		// src video
		// TODO: we should get rid of this and instead allow the user to use the query interface
		//     to visualize the video dataset; creating a new label set doesn't really make sense
		// (we'll still use labels.out_clip_id for the outputs of queries that produce video)
		video := GetVideo(srcVideoID)
		for _, clip := range video.ListClips() {
			db.Exec(
				"INSERT INTO labels (set_id, clip_id, start, end, out_clip_id) VALUES (?, ?, ?, ?, ?)",
				lsID, clip.ID, 0, clip.Frames, clip.ID,
			)
		}
	}

	ls := GetLabelSet(lsID)
	log.Printf("[annotate] created new label set %d", ls.ID)
	return ls, nil
}

func (ls LabelSet) ListLabels() []Label {
	rows := db.Query(
		`SELECT l.id, c.id, c.nframes, c.width, c.height, v.id, v.name, v.ext, l.start, l.end
		FROM labels AS l, clips AS c, videos AS v
		WHERE c.id = l.clip_id AND v.id = c.video_id
		AND l.set_id = ?
		ORDER BY c.id`,
		ls.ID,
	)
	var labels []Label
	for rows.Next() {
		var label Label
		rows.Scan(
			&label.ID, &label.Slice.Clip.ID, &label.Slice.Clip.Frames, &label.Slice.Clip.Width, &label.Slice.Clip.Height,
			&label.Slice.Clip.Video.ID, &label.Slice.Clip.Video.Name, &label.Slice.Clip.Video.Ext,
			&label.Slice.Start, &label.Slice.End,
		)
		label.LabelSet = ls
		labels = append(labels, label)
	}
	return labels
}

func (ls LabelSet) Next() []Image {
	slice := ls.SrcVideo.Uniform(ls.Unit)
	images, err := GetFrames(slice, slice.Clip.Width, slice.Clip.Height)
	if err != nil {
		panic(fmt.Errorf("[annotate] ls.Next: GetFrames: %v", err))
	}
	log.Printf("[annotate] next: got %d frames from labelset %d", len(images), ls.ID)
	return images
}

type LabeledClip struct {
	Slice ClipSlice
	Type DataType
	Label Data
	L *Label
}

func (clipLabel *LabeledClip) GetSlice(start int, end int) *LabeledClip {
	prevSlice := clipLabel.Slice
	slice := ClipSlice{prevSlice.Clip, prevSlice.Start+start, prevSlice.Start+end}
	label := clipLabel.Label.Slice(start, end)
	return &LabeledClip{
		Slice: slice,
		Type: clipLabel.Type,
		Label: label,
	}
}

func (ls LabelSet) Get(index int) *LabeledClip {
	labels := ls.ListLabels()
	if index < 0 || index >= len(labels) {
		return nil
	}
	return labels[index].Load()
}

func (ls LabelSet) GetBySlice(slice ClipSlice) *LabeledClip {
	labels := ls.ListLabels()
	for _, label := range labels {
		if label.Slice.Clip.ID != slice.Clip.ID {
			continue
		}
		if label.Slice.Start > slice.Start || label.Slice.End < slice.End {
			continue
		}
		clipLabel := label.Load()
		return clipLabel.GetSlice(slice.Start-label.Slice.Start, slice.End-label.Slice.Start)
	}
	return nil
}

func (l Label) Load() *LabeledClip {
	t := l.LabelSet.Type
	clipLabel := &LabeledClip{
		Slice: l.Slice,
		L: &l,
		Type: t,
	}

	if t == VideoType {
		// ??? how to load it without loading whole thing

	} else {
		bytes, err := ioutil.ReadFile(fmt.Sprintf("labels/%d/%d.json", l.LabelSet.ID, l.ID))
		if err != nil {
			log.Printf("[annotate] error loading %d/%d: %v", l.LabelSet.ID, l.ID, err)
			return clipLabel
		}
		clipLabel.Label = Data{Type: t}
		if t == DetectionType || t == TrackType {
			if err := json.Unmarshal(bytes, &clipLabel.Label.Detections); err != nil {
				panic(err)
			}
		} else if t == ClassType {
			if err := json.Unmarshal(bytes, &clipLabel.Label.Classes); err != nil {
				panic(err)
			}
		}
	}

	return clipLabel
}

func (ls LabelSet) Length() int {
	// TODO: what we really need is a function that checks which labels
	//       are available on the filesystem
	return len(ls.Video.ListClips())
}

func (ls LabelSet) AddClip(images []Image, label Data) Label {
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
	res := db.Exec("INSERT INTO labels (set_id, clip_id, start, end) VALUES (?, ?, ?, ?)", ls.ID, clip.ID, 0, len(images))
	l := Label{
		ID: res.LastInsertId(),
		Slice: clip.ToSlice(),
		LabelSet: ls,
	}
	ls.UpdateLabel(l, label)
	return l
}

func (ls LabelSet) UpdateLabel(l Label, label Data) {
	bytes, err := json.Marshal(label.Get())
	if err != nil {
		panic(err)
	}
	fname := fmt.Sprintf("labels/%d/%d.json", ls.Video.ID, l.ID)
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

type ClassLabelRequest struct {
	ID int `json:"id"`
	Index int `json:"index"`
	UUID string `json:"uuid"`
	Labels []int `json:"labels"`
}

const VisualizeMaxFrames int = 30*FPS

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
			jsonResponse(w, LabelsResponse{
				URL: fmt.Sprintf("/clips/get?id=%d", clipLabel.Slice.Clip.ID),
				Width: clipLabel.Slice.Clip.Width,
				Height: clipLabel.Slice.Clip.Height,
				Index: index,
				Labels: clipLabel.Label.Get(),
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
			ls.AddClip(images, Data{
				Type: DetectionType,
// TODO				Detections: request.Labels,
			})
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
		ls.UpdateLabel(*clipLabel.L, Data{
			Type: DetectionType,
// TODO			Detections: request.Labels,
		})
	})

	http.HandleFunc("/labelsets/class-label", func(w http.ResponseWriter, r *http.Request) {
		var request ClassLabelRequest
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
				log.Printf("[annotate] cache-label: no item for index=%d, uuid=%s", request.Index, request.UUID)
				w.WriteHeader(400)
				return
			}
			images := item.([]Image)
			log.Printf("[annotate] add labels for new clip to ls %d", ls.ID)
			ls.AddClip(images, Data{
				Type: ClassType,
				Classes: request.Labels,
			})
			w.WriteHeader(200)
			return
		}

		clipLabel := ls.Get(request.Index)
		if clipLabel == nil {
			log.Printf("[annotate] class-label: no clip at index %d", request.Index)
			w.WriteHeader(400)
			return
		}
		log.Printf("[annotate] update labels for clip %d in ls %d", clipLabel.Slice.Clip.ID, ls.ID)
		ls.UpdateLabel(*clipLabel.L, Data{
			Type: ClassType,
			Classes: request.Labels,
		})
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
		end := clipLabel.Slice.Clip.Frames
		if end > VisualizeMaxFrames {
			end = VisualizeMaxFrames
		}
		log.Printf("[annotate] visualize: loading preview for clip %d (frames %d to %d)", clipLabel.Slice.Clip.ID, 0, end)
		pc := clipLabel.GetSlice(0, end).LoadPreview()
		uuid := cache.Add(pc)
		log.Printf("[annotate] visualize: cached preview with %d frames, uuid=%s", pc.Slice.Length(), uuid)
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
