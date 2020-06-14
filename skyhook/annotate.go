package skyhook

import (
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
	OutClipID *int
}

const LabelSetQuery = `
	SELECT ls.id, ls.name, ls.unit, ls.label_type,
		sv.id, sv.name, sv.ext,
		v.id, v.name, v.ext
	FROM label_sets AS ls, videos AS sv, videos AS v
	WHERE sv.id = ls.src_video AND v.id = ls.video_id
`

func labelSetListHelper(rows *Rows) []LabelSet {
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
		videoID := res.LastInsertId()
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
		`SELECT l.id, c.id, c.nframes, c.width, c.height, v.id, v.name, v.ext, l.start, l.end, l.out_clip_id
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
			&label.Slice.Start, &label.Slice.End, &label.OutClipID,
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

func (ls LabelSet) Get(index int) *Label {
	labels := ls.ListLabels()
	if index < 0 || index >= len(labels) {
		return nil
	}
	label := labels[index]
	return &label
}

func (ls LabelSet) GetBySlice(slice ClipSlice) *Label {
	labels := ls.ListLabels()
	for _, label := range labels {
		if label.Slice.Clip.ID != slice.Clip.ID {
			continue
		}
		if label.Slice.Start > slice.Start || label.Slice.End < slice.End {
			continue
		}
		return &label
		/*clipLabel := label.Load()
		return clipLabel.GetSlice(slice.Start-label.Slice.Start, slice.End-label.Slice.Start)*/
	}
	return nil
}

func (l Label) Load(slice ClipSlice) *LabelBuffer {
	if slice.Clip.ID != l.Slice.Clip.ID || slice.Start < l.Slice.Start || slice.End > l.Slice.End {
		return nil
	}
	buf := NewLabelBuffer(l.LabelSet.Type)
	go func() {
		if buf.Type() == VideoType {
			// need to get a slice of the out clip
			outClip := GetClip(*l.OutClipID)
			outSlice := ClipSlice{
				Clip: *outClip,
				Start: slice.Start - l.Slice.Start,
				End: slice.End - l.Slice.Start,
			}
			rd := ReadVideo(outSlice, outSlice.Clip.Width, outSlice.Clip.Height)
			buf.FromVideoReader(rd)
		} else {
			bytes, err := ioutil.ReadFile(fmt.Sprintf("labels/%d/%d.json", l.LabelSet.ID, l.ID))
			if err != nil {
				log.Printf("[annotate] error loading %d/%d: %v", l.LabelSet.ID, l.ID, err)
				buf.Error(err)
				return
			}
			data := Data{Type: buf.Type()}
			if buf.Type() == DetectionType || buf.Type() == TrackType {
				JsonUnmarshal(bytes, &data.Detections)
			} else if buf.Type() == ClassType {
				JsonUnmarshal(bytes, &data.Classes)
			}
			data = data.Slice(slice.Start - l.Slice.Start, slice.End - l.Slice.Start)
			buf.Write(data)
		}
	}()
	return buf
}

func (ls LabelSet) Length() int {
	return len(ls.ListLabels())
}

func (ls LabelSet) AddClip(images []Image, data Data) Label {
	clip := ls.Video.AddClip(len(images), images[0].Width, images[0].Height)
	os.Mkdir(fmt.Sprintf("clips/%d", ls.Video.ID), 0755)
	os.Mkdir(fmt.Sprintf("labels/%d", ls.ID), 0755)
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
	ls.UpdateLabel(l, data)
	return l
}

// Add label without adding a new clip.
// Currently used by exec to store query outputs.
func (ls LabelSet) AddLabel(slice ClipSlice, data Data) Label {
	os.Mkdir(fmt.Sprintf("labels/%d", ls.ID), 0755)
	res := db.Exec("INSERT INTO labels (set_id, clip_id, start, end) VALUES (?, ?, ?, ?)", ls.ID, slice.Clip.ID, slice.Start, slice.End)
	l := Label{
		ID: res.LastInsertId(),
		Slice: slice,
		LabelSet: ls,
	}
	log.Printf("[annotate ls %d] add label for slice %v", ls.ID, slice)
	ls.UpdateLabel(l, data)
	return l
}

func (ls LabelSet) UpdateLabel(l Label, data Data) {
	bytes := JsonMarshal(data.Get())
	fname := fmt.Sprintf("labels/%d/%d.json", l.LabelSet.ID, l.ID)
	if err := ioutil.WriteFile(fname, bytes, 0644); err != nil {
		panic(err)
	}
}

// delete all labels in this set
func (ls LabelSet) Clear() {
	for _, label := range ls.ListLabels() {
		label.Delete()
	}
}

func (l Label) Delete() {
	if l.LabelSet.Type == VideoType {
		// TODO
	} else {
		fname := fmt.Sprintf("labels/%d/%d.json", l.LabelSet.ID, l.ID)
		os.Remove(fname)
		db.Exec("DELETE FROM labels WHERE id = ?", l.ID)
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
	Labels [][]Detection `json:"labels"`
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
	Slice ClipSlice
}

func init() {
	http.HandleFunc("/labelsets", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			JsonResponse(w, ListLabelSets())
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
		JsonResponse(w, ls)
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

		label := ls.Get(index)
		if label != nil {
			data, err := label.Load(label.Slice).ReadFull(label.Slice.Length())
			if err != nil {
				panic(err)
			}
			JsonResponse(w, LabelsResponse{
				URL: fmt.Sprintf("/clips/get?id=%d", label.Slice.Clip.ID),
				Width: label.Slice.Clip.Width,
				Height: label.Slice.Clip.Height,
				Index: index,
				Labels: data.Get(),
			})
			return
		}

		images := ls.Next()
		uuid := cache.Add(images)
		JsonResponse(w, LabelsResponse{
			URL: fmt.Sprintf("/cache/preview?id=%s&type=jpeg", uuid),
			Width: images[0].Width,
			Height: images[0].Height,
			Index: -1,
			UUID: uuid,
		})
	})

	http.HandleFunc("/labelsets/detection-label", func(w http.ResponseWriter, r *http.Request) {
		var request DetectionLabelRequest
		if err := JsonRequest(w, r, &request); err != nil {
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
				Detections: request.Labels,
			})
			w.WriteHeader(200)
			return
		}

		label := ls.Get(request.Index)
		if label == nil {
			log.Printf("[annotate] detection-label: no label at index %d", request.Index)
			w.WriteHeader(400)
			return
		}
		log.Printf("[annotate] update labels for clip %d in ls %d", label.Slice.Clip.ID, ls.ID)
		ls.UpdateLabel(*label, Data{
			Type: DetectionType,
			Detections: request.Labels,
		})
	})

	http.HandleFunc("/labelsets/class-label", func(w http.ResponseWriter, r *http.Request) {
		var request ClassLabelRequest
		if err := JsonRequest(w, r, &request); err != nil {
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

		label := ls.Get(request.Index)
		if label == nil {
			log.Printf("[annotate] class-label: no label at index %d", request.Index)
			w.WriteHeader(400)
			return
		}
		log.Printf("[annotate] update labels for clip %d in ls %d", label.Slice.Clip.ID, ls.ID)
		ls.UpdateLabel(*label, Data{
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
		label := ls.Get(0)
		if label == nil {
			log.Printf("[annotate] visualize: no labels in label set %d", lsID)
			w.WriteHeader(400)
			return
		}
		slice := label.Slice
		if slice.Length() > VisualizeMaxFrames {
			slice.End = slice.Start + VisualizeMaxFrames
		}
		log.Printf("[annotate] visualize: loading preview for slice %v", slice)
		buf := label.Load(slice)
		pc := CreatePreview(slice, buf)
		uuid := cache.Add(pc)
		log.Printf("[annotate] visualize: cached preview with %d frames, uuid=%s", pc.Slice.Length(), uuid)
		JsonResponse(w, VisualizeResponse{
			PreviewURL: fmt.Sprintf("/cache/preview?id=%s&type=jpeg", uuid),
			URL: fmt.Sprintf("/cache/view?id=%s&type=mp4", uuid),
			Width: slice.Clip.Width,
			Height: slice.Clip.Height,
			UUID: uuid,
			Slice: slice,
		})
	})

	/*http.HandleFunc("/labelsets/visualize-more", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		lsID, _ := strconv.Atoi(r.Form.Get("id"))
		clipID, _ := strconv.Atoi(r.Form.Get("clip_id"))
		offset, _ := strconv.Atoi(r.Form.Get("offset"))
	})*/
}
