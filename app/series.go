package app

import (
	"../vaas"

	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
)

type DBTimeline struct {vaas.Timeline}
type DBSegment struct {vaas.Segment}
type DBSeries struct {
	vaas.Series

	loaded bool
	SrcVectorStr *string
	NodeID *int
}
type DBItem struct{vaas.Item}

const TimelineQuery = "SELECT id, name FROM timelines"

func timelineListHelper(rows *Rows) []DBTimeline {
	timelines := []DBTimeline{}
	for rows.Next() {
		var timeline DBTimeline
		rows.Scan(&timeline.ID, &timeline.Name)
		timelines = append(timelines, timeline)
	}
	return timelines
}

func ListTimelines() []DBTimeline {
	rows := db.Query(TimelineQuery)
	return timelineListHelper(rows)
}

func GetTimeline(id int) *DBTimeline {
	rows := db.Query(TimelineQuery + " WHERE id = ?", id)
	timelines := timelineListHelper(rows)
	if len(timelines) == 1 {
		timeline := timelines[0]
		return &timeline
	} else {
		return nil
	}
}

const SeriesQuery = "SELECT id, timeline_id, name, type, data_type, src_vector, node_id, percent FROM series"

func seriesListHelper(rows *Rows) []*DBSeries {
	series := []*DBSeries{}
	for rows.Next() {
		var s DBSeries
		rows.Scan(&s.ID, &s.Timeline.ID, &s.Name, &s.Type, &s.DataType, &s.SrcVectorStr, &s.NodeID, &s.Percent)
		series = append(series, &s)
	}
	for _, s := range series {
		s.Timeline = GetTimeline(s.Timeline.ID).Timeline
	}
	return series
}

func ListSeries() []*DBSeries {
	rows := db.Query(SeriesQuery)
	return seriesListHelper(rows)
}

func ListSeriesByType(t string) []*DBSeries {
	rows := db.Query(SeriesQuery + " WHERE type = ?", t)
	return seriesListHelper(rows)
}

func GetSeries(id int) *DBSeries {
	rows := db.Query(SeriesQuery + " WHERE id = ?", id)
	series := seriesListHelper(rows)
	if len(series) == 1 {
		return series[0]
	} else {
		return nil
	}
}

func (series *DBSeries) Load() {
	if series.loaded {
		return
	}

	if series.SrcVectorStr != nil {
		series.SrcVector = ParseVector(*series.SrcVectorStr).List()
	}
	if series.NodeID != nil {
		series.Node = &GetNode(*series.NodeID).Node
	}
	series.loaded = true
}

type Vector []*DBSeries

func ParseVector(str string) Vector {
	var vector Vector
	for _, part := range strings.Split(str, ",") {
		id := vaas.ParseInt(part)
		vector = append(vector, GetSeries(id))
	}
	if len(vector) == 0 {
		panic(fmt.Errorf("vector cannot be empty"))
	}
	return vector
}

func (v Vector) String() string {
	if len(v) == 0 {
		panic(fmt.Errorf("vector cannot be empty"))
	}
	var parts []string
	for _, series := range v {
		parts = append(parts, fmt.Sprintf("%d", series.ID))
	}
	return strings.Join(parts, ",")
}

func (v Vector) List() []vaas.Series {
	l := []vaas.Series{}
	for _, series := range v {
		l = append(l, series.Series)
	}
	return l
}

func VectorFromList(l []vaas.Series) Vector {
	var vector Vector
	for _, series := range l {
		vector = append(vector, GetSeries(series.ID))
	}
	return vector
}

const SegmentQuery = "SELECT id, timeline_id, name, frames, fps FROM segments"

func segmentListHelper(rows *Rows) []DBSegment {
	var segments []DBSegment
	for rows.Next() {
		var s DBSegment
		rows.Scan(&s.ID, &s.Timeline.ID, &s.Name, &s.Frames, &s.FPS)
		segments = append(segments, s)
	}
	for i := range segments {
		timeline := GetTimeline(segments[i].Timeline.ID)
		segments[i].Timeline = timeline.Timeline
	}
	return segments
}

func (timeline DBTimeline) ListSegments() []DBSegment {
	rows := db.Query(SegmentQuery + " WHERE timeline_id = ?", timeline.ID)
	return segmentListHelper(rows)
}

func GetSegment(id int) *DBSegment {
	rows := db.Query(SegmentQuery + " WHERE id = ?", id)
	segments := segmentListHelper(rows)
	if len(segments) == 1 {
		s := segments[0]
		return &s
	} else {
		return nil
	}
}

func (timeline DBTimeline) AddSegment(name string, frames int, fps int) *DBSegment {
	res := db.Exec(
		"INSERT INTO segments (timeline_id, name, frames, fps) VALUES (?, ?, ?, ?)",
		timeline.ID, name, frames, fps,
	)
	return GetSegment(res.LastInsertId())
}

func (timeline DBTimeline) Uniform(unit int) vaas.Slice {
	segments := timeline.ListSegments()

	// select a segment
	segment := func() DBSegment {
		if unit == 0 {
			return segments[rand.Intn(len(segments))]
		}
		weights := make([]int, len(segments))
		sum := 0
		for i, segment := range segments {
			weights[i] = (segment.Frames+unit-1) / unit
			if weights[i] < 0 {
				weights[i] = 0
			}
			sum += weights[i]
		}
		r := rand.Intn(sum)
		for i, segment := range segments {
			r -= weights[i]
			if r <= 0 {
				return segment
			}
		}
		return segments[len(segments)-1]
	}()

	// select frame
	var start, end int
	if unit == 0 {
		start = 0
		end = segment.Frames
	} else {
		idx := rand.Intn((segment.Frames+unit-1)/unit)
		start = idx*unit
		end = (idx+1)*unit
		if end > segment.Frames {
			end = segment.Frames
		}
	}

	return vaas.Slice{segment.Segment, start, end}
}

func (series DBSeries) Clear() {
	err := os.RemoveAll(fmt.Sprintf("items/%d", series.ID))
	if err != nil {
		log.Printf("[series] warning: error clearing series %s (id=%d): %v", series.Name, series.ID, err)
	}
	db.Exec("DELETE FROM items WHERE series_id = ?", series.ID)
}

func (series DBSeries) Delete() {
	series.Clear()
	db.Exec("DELETE FROM series WHERE id = ?", series.ID)
}

func (series DBSeries) Next() vaas.Slice {
	return DBTimeline{series.Timeline}.Uniform(1)
}

const ItemQuery = "SELECT id, segment_id, series_id, start, end, format, width, height, freq FROM items"

func itemListHelper(rows *Rows) []DBItem {
	var items []DBItem
	for rows.Next() {
		var item DBItem
		rows.Scan(&item.ID, &item.Slice.Segment.ID, &item.Series.ID, &item.Slice.Start, &item.Slice.End, &item.Format, &item.Width, &item.Height, &item.Freq)
		items = append(items, item)
	}
	for i := range items {
		segment := GetSegment(items[i].Slice.Segment.ID)
		series := GetSeries(items[i].Series.ID)
		items[i].Slice.Segment = segment.Segment
		items[i].Series = series.Series
	}
	return items
}

func (series DBSeries) ListItems() []DBItem {
	rows := db.Query(ItemQuery + " WHERE series_id = ? ORDER BY id", series.ID)
	return itemListHelper(rows)
}

func GetItem(id int) *DBItem {
	rows := db.Query(ItemQuery + " WHERE id = ?", id)
	items := itemListHelper(rows)
	if len(items) == 1 {
		item := items[0]
		return &item
	} else {
		return nil
	}
}

func (series DBSeries) AddItem(slice vaas.Slice, format string, videoDims [2]int, freq int) *DBItem {
	res := db.Exec(
		"INSERT INTO items (segment_id, series_id, start, end, format, width, height, freq) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		slice.Segment.ID, series.ID, slice.Start, slice.End, format, videoDims[0], videoDims[1], freq,
	)
	return GetItem(res.LastInsertId())
}

func (series DBSeries) GetItem(slice vaas.Slice) *DBItem {
	rows := db.Query(ItemQuery + " WHERE series_id = ? AND segment_id = ? AND start <= ? AND end >= ? LIMIT 1", series.ID, slice.Segment.ID, slice.Start, slice.End)
	items := itemListHelper(rows)
	if len(items) == 1 {
		item := items[0]
		return &item
	} else {
		return nil
	}
}

// Returns data for the specified slice.
// For output series, computes the data if needed.
// Currently the computed data is persisted so that we didn't need to change how DataRefs work.
// (Export will use the persisted data.)
func (series *DBSeries) RequireData(slice vaas.Slice) (vaas.DataBuffer, error) {
	series.Load()
	item := series.GetItem(slice)
	if item != nil {
		return item.Load(slice), nil
	}
	if series.Node == nil {
		return nil, fmt.Errorf("series %s has no item for slice %v", series.Name, slice)
	}
	query := GetQuery(series.Node.QueryID)
	vector := VectorFromList(series.SrcVector)
	context := query.Allocate(vector, slice)
	context.Opts = vaas.ExecOptions{PersistVideo: true}
	buf, err := context.GetBuffer(*series.Node)
	context.Release()
	return buf, err
}

func (series DBSeries) Get(index int) *DBItem {
	items := series.ListItems()
	if index < 0 || index >= len(items) {
		return nil
	}
	item := items[index]
	return &item
}

func (series DBSeries) Length() int {
	return len(series.ListItems())
}

// Add label without adding a new clip.
// Currently used by exec to store query outputs.
func (series DBSeries) WriteItem(slice vaas.Slice, data vaas.Data, freq int) *DBItem {
	os.Mkdir(fmt.Sprintf("items/%d", series.ID), 0755)
	item := series.AddItem(slice, "json", [2]int{0, 0}, freq)
	log.Printf("[annotate series %d] add item %d for slice %v", series.ID, item.ID, slice)
	item.UpdateData(data)
	return item
}

func (item DBItem) Delete() {
	if item.Format == "jpeg" {
		os.RemoveAll(fmt.Sprintf("items/%d/%d/", item.Series.ID, item.ID))
	} else {
		os.Remove(item.Fname(0))
	}
	db.Exec("DELETE FROM items WHERE id = ?", item.ID)
}

func init() {
	http.HandleFunc("/datasets", func(w http.ResponseWriter, r *http.Request) {
		vaas.JsonResponse(w, ListSeriesByType("data"))
	})

	http.HandleFunc("/series", func(w http.ResponseWriter, r *http.Request) {
		vaas.JsonResponse(w, ListSeries())
	})

	http.HandleFunc("/series/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		r.ParseForm()
		seriesID := vaas.ParseInt(r.PostForm.Get("series_id"))
		series := GetSeries(seriesID)
		if series == nil {
			w.WriteHeader(404)
			return
		}
		series.Delete()
	})

	http.HandleFunc("/series/random-slice", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		seriesID := vaas.ParseInt(r.PostForm.Get("series_id"))
		unit := vaas.ParseInt(r.PostForm.Get("unit"))
		series := GetSeries(seriesID)
		if series == nil {
			http.Error(w, "no such series", 404)
			return
		}
		slice := DBTimeline{Timeline: series.Timeline}.Uniform(unit)
		vaas.JsonResponse(w, slice)
	})

	http.HandleFunc("/series/get-item", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		seriesID := vaas.ParseInt(r.Form.Get("series_id"))
		segmentID := vaas.ParseInt(r.Form.Get("segment_id"))
		start := vaas.ParseInt(r.Form.Get("start"))
		end := vaas.ParseInt(r.Form.Get("end"))
		contentType := r.Form.Get("type")

		series := GetSeries(seriesID)
		if series == nil {
			http.Error(w, "no such series", 404)
			return
		}
		segment := GetSegment(segmentID)
		if segment == nil {
			http.Error(w, "no such segment", 404)
			return
		}
		slice := vaas.Slice{segment.Segment, start, end}
		item := series.GetItem(slice)
		if item == nil {
			http.Error(w, "no matching item", 404)
			return
		}
		if contentType == "jpeg" || contentType == "mp4" {
			rd := vaas.ReadVideo(item.Item, slice, vaas.ReadVideoOptions{})
			defer rd.Close()
			if contentType == "jpeg" {
				// return image
				im, err := rd.Read()
				if err != nil {
					log.Printf("[/series/get-item] error reading frame (item %d): %v", item.ID, err)
					http.Error(w, "error reading frame", 400)
					return
				}
				w.Header().Set("Content-Type", "image/jpeg")
				w.Write(im.AsJPG())
			} else if contentType == "mp4" {
				vout, cmd := vaas.MakeVideo(rd, item.Width, item.Height)
				_, err := io.Copy(w, vout)
				if err != nil {
					log.Printf("[/series/get-item] error reading video (item %d): %v", item.ID, err)
				}
				cmd.Wait()
			}
		} else if contentType == "json" {
			data := item.Load(slice)
			vaas.JsonResponse(w, data)
		} else if contentType == "meta" {
			vaas.JsonResponse(w, item)
		}
	})

	// called from container
	http.HandleFunc("/series/add-output-item", func(w http.ResponseWriter, r *http.Request) {
		var request vaas.AddOutputItemRequest
		if err := vaas.ParseJsonRequest(w, r, &request); err != nil {
			return
		}

		// if no outputs series, create one to store the exec outputs
		node := &DBNode{Node: request.Node}
		vector := VectorFromList(request.Vector)
		vn := GetOrCreateVNode(node, vector)
		vn.EnsureSeries()
		item := DBSeries{Series: *vn.Series}.AddItem(request.Slice, request.Format, request.Dims, request.Freq)
		vaas.JsonResponse(w, item)
	})
}
