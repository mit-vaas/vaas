package app

import (
	"../vaas"

	"fmt"
	"log"
	"net/http"
)

func NewLabelSeries(name string, src []*DBSeries, dataType vaas.DataType, metadata string) (*DBSeries, error) {
	if dataType != vaas.DetectionType && dataType != vaas.TrackType && dataType != vaas.ClassType && dataType != vaas.VideoType {
		return nil, fmt.Errorf("invalid data type %s", dataType)
	}

	// get timeline ID and src vector string
	timeline := src[0].Timeline
	for _, s := range src {
		if s.Timeline.ID != timeline.ID {
			return nil, fmt.Errorf("source series must have the same timeline")
		}
	}

	res := db.Exec(
			"INSERT INTO series (timeline_id, name, type, data_type, src_vector, annotate_metadata) VALUES (?, ?, 'labels', ?, ?, ?)",
			timeline.ID, name, dataType, Vector(src).String(), metadata,
	)
	series := GetSeries(res.LastInsertId())
	log.Printf("[annotate] created new labels series %d", series.ID)
	return series, nil
}

type LabelsResponse struct {
	// URLs for the source series (cached DataBuffers)
	URLs []string
	// Index in the label series of this item, or -1 if we're labeling something new.
	Index int
	// Slice that we are labeling
	Slice vaas.Slice
	// If Index != -1, this contains the encoded annotation data.
	Labels interface{}
}

type DetectionLabelRequest struct {
	ID int `json:"id"`
	Index int `json:"index"`
	Slice vaas.Slice `json:"slice"`
	Labels [][]vaas.Detection `json:"labels"`
}

type ClassLabelRequest struct {
	ID int `json:"id"`
	Index int `json:"index"`
	Slice vaas.Slice `json:"slice"`
	Labels []int `json:"labels"`
}

const VisualizeMaxFrames int = 30*vaas.FPS

type VisualizeResponse struct {
	PreviewURL string
	URL string
	Width int
	Height int
	UUID string
	Slice vaas.Slice
	Type vaas.DataType
	Vectors [][]*DBSeries
}

func init() {
	http.HandleFunc("/labelseries", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			l := []vaas.Series{}
			for _, series := range ListSeriesByType("labels") {
				series.Load()
				l = append(l, series.Series)
			}
			vaas.JsonResponse(w, l)
			return
		} else if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		r.ParseForm()
		name := r.PostForm.Get("name")
		labelType := r.PostForm.Get("type")
		srcVector := ParseVector(r.PostForm.Get("src"))
		metadata := r.PostForm.Get("metadata")
		series, err := NewLabelSeries(name, srcVector, vaas.DataType(labelType), metadata)
		if err != nil {
			w.WriteHeader(400)
			return
		}
		vaas.JsonResponse(w, series)
	})

	http.HandleFunc("/series/labels", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		id := vaas.ParseInt(r.Form.Get("id"))
		index := vaas.ParseInt(r.Form.Get("index"))
		numFrames := 1
		if r.Form["nframes"] != nil {
			numFrames = vaas.ParseInt(r.Form.Get("nframes"))
		}
		series := GetSeries(id)
		if series == nil {
			log.Printf("[annotate] no series with id %d", id)
			w.WriteHeader(400)
			return
		}
		series.Load()

		// get labels for existing item, or sample a new slice
		var resp LabelsResponse
		item := series.Get(index)
		if item != nil {
			data, err := item.Load(item.Slice).Reader().Read(item.Slice.Length())
			if err != nil {
				panic(err)
			}
			resp.Index = index
			resp.Slice = item.Slice
			resp.Labels = data
		} else {
			resp.Index = -1
			resp.Slice = series.Next(numFrames)
		}

		// fetch buffers for the source series
		resp.URLs = make([]string, len(series.SrcVector))
		for i, src := range VectorFromList(series.SrcVector) {
			buf, err := src.RequireData(resp.Slice)
			if err != nil {
				panic(err)
			}
			cacheID := cache.Add(&CachedDataBuffer{
				Buf: buf,
				Slice: resp.Slice,
			})
			resp.URLs[i] = fmt.Sprintf("/cache/view?id=%s", cacheID)
		}

		vaas.JsonResponse(w, resp)
	})

	http.HandleFunc("/series/detection-label", func(w http.ResponseWriter, r *http.Request) {
		var request DetectionLabelRequest
		if err := vaas.ParseJsonRequest(w, r, &request); err != nil {
			return
		}

		series := GetSeries(request.ID)
		if series == nil {
			log.Printf("[annotate] no series with id %d", request.ID)
			http.Error(w, "no such series", 400)
			return
		}

		if request.Index == -1 {
			segment := GetSegment(request.Slice.Segment.ID)
			slice := vaas.Slice{segment.Segment, request.Slice.Start, request.Slice.End}
			series.WriteItem(slice, vaas.DetectionData(request.Labels), 1)
			log.Printf("[annotate] add new label item to series %d", series.ID)
			w.WriteHeader(200)
			return
		}

		item := series.Get(request.Index)
		if item == nil {
			log.Printf("[annotate] detection-label: no item at index %d", request.Index)
			http.Error(w, "no such item", 400)
			return
		}
		log.Printf("[annotate] update item for %v in series %d", item.Slice, series.ID)
		item.UpdateData(vaas.DetectionData(request.Labels))
	})

	http.HandleFunc("/series/class-label", func(w http.ResponseWriter, r *http.Request) {
		var request ClassLabelRequest
		if err := vaas.ParseJsonRequest(w, r, &request); err != nil {
			return
		}

		series := GetSeries(request.ID)
		if series == nil {
			log.Printf("[annotate] no series with id %d", request.ID)
			http.Error(w, "no such series", 400)
			return
		}

		if request.Index == -1 {
			segment := GetSegment(request.Slice.Segment.ID)
			slice := vaas.Slice{segment.Segment, request.Slice.Start, request.Slice.End}
			series.WriteItem(slice, vaas.ClassData(request.Labels), 1)
			log.Printf("[annotate] add new label item to series %d", series.ID)
			w.WriteHeader(200)
			return
		}

		item := series.Get(request.Index)
		if item == nil {
			log.Printf("[annotate] class-label: no item at index %d", request.Index)
			http.Error(w, "no such item", 400)
			return
		}
		log.Printf("[annotate] update item for %v in series %d", item.Slice, series.ID)
		item.UpdateData(vaas.ClassData(request.Labels))
	})

	http.HandleFunc("/labelsets/visualize", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		id := vaas.ParseInt(r.Form.Get("id"))
		series := GetSeries(id)
		if series == nil {
			log.Printf("[annotate] visualize: no labels series %d", series.ID)
			http.Error(w, "no such series", 404)
			return
		}
		series.Load()
		background := series.SrcVector[0]

		// TODO: we should return some kind of data that client can use to access
		//       additional clips via visualize-more; currently they will only be
		//       able to see the first 30 sec in the first clip
		log.Printf("[annotate] visualize: loading series item %d/%d", series.ID, 0)
		item := series.Get(0)
		if item == nil {
			log.Printf("[annotate] visualize: no items in series %d", series.ID)
			http.Error(w, "no such item", 404)
			return
		}
		slice := item.Slice
		if slice.Length() > VisualizeMaxFrames {
			slice.End = slice.Start + VisualizeMaxFrames
		}
		log.Printf("[annotate] visualize: rendering video for slice %v", slice)
		bgItem := DBSeries{Series: background}.GetItem(slice)
		inputs := [][]vaas.DataReader{{
			vaas.VideoFileBuffer{bgItem.Item, slice}.Reader(),
			item.Load(slice).Reader(),
		}}
		renderer := RenderVideo(slice, inputs, RenderOpts{})
		uuid := cache.Add(renderer)
		log.Printf("[annotate] visualize: cached renderer with %d frames, uuid=%s", slice.Length(), uuid)
		vaas.JsonResponse(w, VisualizeResponse{
			PreviewURL: fmt.Sprintf("/cache/preview?id=%s&type=jpeg", uuid),
			URL: fmt.Sprintf("/cache/view?id=%s&type=mp4", uuid),
			Width: bgItem.Width,
			Height: bgItem.Height,
			UUID: uuid,
			Slice: slice,
			Type: series.DataType,
		})
	})

	/*http.HandleFunc("/labelsets/visualize-more", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		lsID, _ := strconv.Atoi(r.Form.Get("id"))
		clipID, _ := strconv.Atoi(r.Form.Get("clip_id"))
		offset, _ := strconv.Atoi(r.Form.Get("offset"))
	})*/
}
