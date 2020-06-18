package skyhook

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)

func NewLabelSeries(name string, src []*Series, dataType DataType) (*Series, error) {
	if dataType != DetectionType && dataType != TrackType && dataType != ClassType && dataType != VideoType {
		return nil, fmt.Errorf("invalid data type %s", dataType)
	}

	// get timeline ID and src vector string
	var vectorParts []string
	timeline := src[0].Timeline
	for _, s := range src {
		if s.Timeline.ID != timeline.ID {
			return nil, fmt.Errorf("source series must have the same timeline")
		}
		vectorParts = append(vectorParts, fmt.Sprintf("%d", s.ID))
	}
	vectorStr := strings.Join(vectorParts, ",")

	res := db.Exec(
			"INSERT INTO series (timeline_id, name, type, data_type, src_vector) VALUES (?, ?, 'labels', ?, ?)",
			timeline.ID, name, dataType, vectorStr,
	)
	series := GetSeries(res.LastInsertId())
	log.Printf("[annotate] created new labels series %d", series.ID)
	return series, nil
}

func (series *Series) Next() Slice {
	return series.Timeline.Uniform(1)
}

func (item Item) Load(slice Slice) DataBuffer {
	if slice.Segment.ID != item.Slice.Segment.ID || slice.Start < item.Slice.Start || slice.End > item.Slice.End {
		return nil
	}
	if item.Series.DataType == VideoType {
		return VideoFileBuffer{item, slice}
	}
	buf := NewSimpleBuffer(item.Series.DataType)
	go func() {
		bytes, err := ioutil.ReadFile(item.Fname(0))
		if err != nil {
			log.Printf("[annotate] error loading %d/%d: %v", item.Series.ID, item.ID, err)
			buf.Error(err)
			return
		}
		data := DecodeData(item.Series.DataType, bytes)
		data = data.Slice(slice.Start - item.Slice.Start, slice.End - item.Slice.Start)
		buf.Write(data)
		buf.Close()
	}()
	return buf
}

func (series Series) Get(index int) *Item {
	items := series.ListItems()
	if index < 0 || index >= len(items) {
		return nil
	}
	item := items[index]
	return &item
}

func (series Series) Length() int {
	return len(series.ListItems())
}

// Add label without adding a new clip.
// Currently used by exec to store query outputs.
func (series Series) WriteItem(slice Slice, data Data) *Item {
	os.Mkdir(fmt.Sprintf("items/%d", series.ID), 0755)
	item := series.AddItem(slice, "json", [2]int{0, 0})
	log.Printf("[annotate series %d] add item %d for slice %v", series.ID, item.ID, slice)
	item.UpdateData(data)
	return item
}

func (item Item) UpdateData(data Data) {
	if err := ioutil.WriteFile(item.Fname(0), data.Encode(), 0644); err != nil {
		panic(err)
	}
}

func (item Item) Delete() {
	if item.Format == "jpeg" {
		os.RemoveAll(fmt.Sprintf("items/%d/%d/", item.Series.ID, item.ID))
	} else {
		os.Remove(item.Fname(0))
	}
	db.Exec("DELETE FROM items WHERE id = ?", item.ID)
}

type LabelsResponse struct {
	URL string
	Width int
	Height int
	Index int
	Slice Slice
	Labels interface{}
}

type DetectionLabelRequest struct {
	ID int `json:"id"`
	Index int `json:"index"`
	Slice Slice `json:"slice"`
	Labels [][]Detection `json:"labels"`
}

type ClassLabelRequest struct {
	ID int `json:"id"`
	Index int `json:"index"`
	Slice Slice `json:"slice"`
	Labels []int `json:"labels"`
}

const VisualizeMaxFrames int = 30*FPS

type VisualizeResponse struct {
	PreviewURL string
	URL string
	Width int
	Height int
	UUID string
	Slice Slice
	Type DataType
	Vectors [][]*Series
}

func init() {
	http.HandleFunc("/labelseries", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			l := []*Series{}
			for _, series := range ListSeriesByType("labels") {
				series.Load()
				l = append(l, series)
			}
			JsonResponse(w, l)
			return
		} else if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		r.ParseForm()
		name := r.PostForm.Get("name")
		labelType := r.PostForm.Get("type")
		srcVector := ParseVector(r.PostForm.Get("src"))
		series, err := NewLabelSeries(name, srcVector, DataType(labelType))
		if err != nil {
			w.WriteHeader(400)
			return
		}
		JsonResponse(w, series)
	})

	http.HandleFunc("/series/labels", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		id := ParseInt(r.Form.Get("id"))
		index := ParseInt(r.Form.Get("index"))
		series := GetSeries(id)
		if series == nil {
			log.Printf("[annotate] no series with id %d", id)
			w.WriteHeader(400)
			return
		}
		series.Load()
		background := series.SrcVector[0]

		item := series.Get(index)
		if item != nil {
			data, err := item.Load(item.Slice).Reader().Read(item.Slice.Length())
			if err != nil {
				panic(err)
			}
			bgItem := background.GetItem(item.Slice)
			JsonResponse(w, LabelsResponse{
				URL: fmt.Sprintf("/series/get-item?series_id=%d&segment_id=%d&start=%d&end=%d", background.ID, item.Slice.Segment.ID, item.Slice.Start, item.Slice.End),
				Width: bgItem.Width,
				Height: bgItem.Height,
				Index: index,
				Labels: data,
			})
			return
		}

		slice := series.Next()
		bgItem := background.GetItem(slice)
		JsonResponse(w, LabelsResponse{
			URL: fmt.Sprintf("/series/get-item?series_id=%d&segment_id=%d&start=%d&end=%d", background.ID, slice.Segment.ID, slice.Start, slice.End),
			Width: bgItem.Width,
			Height: bgItem.Height,
			Index: -1,
			Slice: slice,
		})
	})

	http.HandleFunc("/series/detection-label", func(w http.ResponseWriter, r *http.Request) {
		var request DetectionLabelRequest
		if err := JsonRequest(w, r, &request); err != nil {
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
			slice := Slice{*segment, request.Slice.Start, request.Slice.End}
			series.WriteItem(slice, DetectionData(request.Labels))
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
		item.UpdateData(DetectionData(request.Labels))
	})

	http.HandleFunc("/series/class-label", func(w http.ResponseWriter, r *http.Request) {
		var request ClassLabelRequest
		if err := JsonRequest(w, r, &request); err != nil {
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
			slice := Slice{*segment, request.Slice.Start, request.Slice.End}
			series.WriteItem(slice, ClassData(request.Labels))
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
		item.UpdateData(ClassData(request.Labels))
	})

	http.HandleFunc("/labelsets/visualize", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		id := ParseInt(r.Form.Get("id"))
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
		bgItem := background.GetItem(slice)
		inputs := [][]DataReader{{
			VideoFileBuffer{*bgItem, slice}.Reader(),
			item.Load(slice).Reader(),
		}}
		renderer := RenderVideo(slice, inputs, RenderOpts{})
		uuid := cache.Add(renderer)
		log.Printf("[annotate] visualize: cached renderer with %d frames, uuid=%s", slice.Length(), uuid)
		JsonResponse(w, VisualizeResponse{
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
