package skyhook

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
)

// we assume all videos are constant 25 fps
// TODO: use per-clip or per-video fps
// re-encoding is likely needed to work with skyhook (if video has variable framerate)
// ffmpeg seeking is only fast and frame-accurate with constant framerate
const FPS int = 25

type Timeline struct {
	ID int
	Name string
}

type Series struct {
	ID int
	Timeline Timeline
	Name string
	Type string
	DataType DataType

	// for labels/outputs
	SrcVectorStr *string
	// for outputs
	NodeID *int
	// for data, during ingestion
	Percent int

	SrcVector []*Series
	Node *Node
}

type Segment struct {
	ID int
	Timeline Timeline
	Name string
	Frames int
	FPS float64
}

type Slice struct {
	Segment Segment
	Start int
	End int
}

type Item struct {
	ID int
	Slice Slice
	Series *Series
	Format string

	Width int
	Height int
}

func (slice Slice) String() string {
	return fmt.Sprintf("%d[%d:%d]", slice.Segment.ID, slice.Start, slice.End)
}

func (slice Slice) Length() int {
	return slice.End - slice.Start
}

const TimelineQuery = "SELECT id, name FROM timelines"

func timelineListHelper(rows *Rows) []Timeline {
	timelines := []Timeline{}
	for rows.Next() {
		var timeline Timeline
		rows.Scan(&timeline.ID, &timeline.Name)
		timelines = append(timelines, timeline)
	}
	return timelines
}

func ListTimelines() []Timeline {
	rows := db.Query(TimelineQuery)
	return timelineListHelper(rows)
}

func GetTimeline(id int) *Timeline {
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

func seriesListHelper(rows *Rows) []*Series {
	series := []*Series{}
	for rows.Next() {
		var s Series
		rows.Scan(&s.ID, &s.Timeline.ID, &s.Name, &s.Type, &s.DataType, &s.SrcVectorStr, &s.NodeID, &s.Percent)
		series = append(series, &s)
	}
	for _, s := range series {
		timeline := GetTimeline(s.Timeline.ID)
		s.Timeline = *timeline
	}
	return series
}

func ListSeries() []*Series {
	rows := db.Query(SeriesQuery)
	return seriesListHelper(rows)
}

func ListSeriesByType(t string) []*Series {
	rows := db.Query(SeriesQuery + " WHERE type = ?", t)
	return seriesListHelper(rows)
}

func GetSeries(id int) *Series {
	rows := db.Query(SeriesQuery + " WHERE id = ?", id)
	series := seriesListHelper(rows)
	if len(series) == 1 {
		return series[0]
	} else {
		return nil
	}
}

func (series *Series) Load() {
	if series.SrcVectorStr != nil {
		series.SrcVector = ParseVector(*series.SrcVectorStr)
	}
	if series.NodeID != nil {
		series.Node = GetNode(*series.NodeID)
	}
}

type Vector []*Series

func ParseVector(str string) Vector {
	var vector Vector
	for _, part := range strings.Split(str, ",") {
		id := ParseInt(part)
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

const SegmentQuery = "SELECT id, timeline_id, name, frames, fps FROM segments"

func segmentListHelper(rows *Rows) []Segment {
	var segments []Segment
	for rows.Next() {
		var s Segment
		rows.Scan(&s.ID, &s.Timeline.ID, &s.Name, &s.Frames, &s.FPS)
		segments = append(segments, s)
	}
	for i := range segments {
		timeline := GetTimeline(segments[i].Timeline.ID)
		segments[i].Timeline = *timeline
	}
	return segments
}

func (timeline Timeline) ListSegments() []Segment {
	rows := db.Query(SegmentQuery + " WHERE timeline_id = ?", timeline.ID)
	return segmentListHelper(rows)
}

func GetSegment(id int) *Segment {
	rows := db.Query(SegmentQuery + " WHERE id = ?", id)
	segments := segmentListHelper(rows)
	if len(segments) == 1 {
		s := segments[0]
		return &s
	} else {
		return nil
	}
}

func (timeline Timeline) AddSegment(name string, frames int, fps int) *Segment {
	res := db.Exec(
		"INSERT INTO segments (timeline_id, name, frames, fps) VALUES (?, ?, ?, ?)",
		timeline.ID, name, frames, fps,
	)
	return GetSegment(res.LastInsertId())
}

func (timeline Timeline) Uniform(unit int) Slice {
	segments := timeline.ListSegments()

	// select a segment
	segment := func() Segment {
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

	return Slice{segment, start, end}
}

func (series Series) Clear() {
	err := os.RemoveAll(fmt.Sprintf("items/%d", series.ID))
	if err != nil {
		log.Printf("[series] warning: error clearing series %s (id=%d): %v", series.Name, series.ID, err)
	}
	db.Exec("DELETE FROM items WHERE series_id = ?", series.ID)
}

func (series Series) Delete() {
	series.Clear()
	db.Exec("DELETE FROM series WHERE id = ?", series.ID)
}

func pad6(x int) string {
	s := fmt.Sprintf("%d", x)
	for len(s) < 6 {
		s = "0" + s
	}
	return s
}

const ItemQuery = "SELECT id, segment_id, series_id, start, end, format, width, height FROM items"

func itemListHelper(rows *Rows) []Item {
	var items []Item
	for rows.Next() {
		item := Item{Series: &Series{}}
		rows.Scan(&item.ID, &item.Slice.Segment.ID, &item.Series.ID, &item.Slice.Start, &item.Slice.End, &item.Format, &item.Width, &item.Height)
		items = append(items, item)
	}
	for i := range items {
		segment := GetSegment(items[i].Slice.Segment.ID)
		series := GetSeries(items[i].Series.ID)
		items[i].Slice.Segment = *segment
		items[i].Series = series
	}
	return items
}

func (series Series) ListItems() []Item {
	rows := db.Query(ItemQuery + " WHERE series_id = ? ORDER BY id", series.ID)
	return itemListHelper(rows)
}

func GetItem(id int) *Item {
	rows := db.Query(ItemQuery + " WHERE id = ?", id)
	items := itemListHelper(rows)
	if len(items) == 1 {
		item := items[0]
		return &item
	} else {
		return nil
	}
}

func (series Series) AddItem(slice Slice, format string, videoDims [2]int) *Item {
	res := db.Exec(
		"INSERT INTO items (segment_id, series_id, start, end, format, width, height) VALUES (?, ?, ?, ?, ?, ?, ?)",
		slice.Segment.ID, series.ID, slice.Start, slice.End, format, videoDims[0], videoDims[1],
	)
	return GetItem(res.LastInsertId())
}

func (series Series) GetItem(slice Slice) *Item {
	rows := db.Query(ItemQuery + " WHERE series_id = ? AND segment_id = ? AND start <= ? AND end >= ? LIMIT 1", series.ID, slice.Segment.ID, slice.Start, slice.End)
	items := itemListHelper(rows)
	if len(items) == 1 {
		item := items[0]
		return &item
	} else {
		return nil
	}
}

func (item Item) Fname(index int) string {
	if item.Format == "jpeg" {
		return fmt.Sprintf("items/%d/%d/%s.jpg", item.Series.ID, item.ID, pad6(index))
	} else {
		// json / mp4 (and other video formats)
		return fmt.Sprintf("items/%d/%d.%s", item.Series.ID, item.ID, item.Format)
	}
}

func (segment Segment) ToSlice() Slice {
	return Slice{segment, 0, segment.Frames}
}

func init() {
	http.HandleFunc("/datasets", func(w http.ResponseWriter, r *http.Request) {
		JsonResponse(w, ListSeriesByType("data"))
	})

	http.HandleFunc("/series/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		r.ParseForm()
		seriesID := ParseInt(r.PostForm.Get("series_id"))
		series := GetSeries(seriesID)
		if series == nil {
			w.WriteHeader(404)
			return
		}
		series.Delete()
	})

	http.HandleFunc("/series/random-slice", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		seriesID := ParseInt(r.PostForm.Get("series_id"))
		unit := ParseInt(r.PostForm.Get("unit"))
		series := GetSeries(seriesID)
		if series == nil {
			http.Error(w, "no such series", 404)
			return
		}
		slice := series.Timeline.Uniform(unit)
		JsonResponse(w, slice)
	})

	http.HandleFunc("/series/get-item", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		seriesID := ParseInt(r.Form.Get("series_id"))
		segmentID := ParseInt(r.Form.Get("segment_id"))
		start := ParseInt(r.Form.Get("start"))
		end := ParseInt(r.Form.Get("end"))
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
		slice := Slice{*segment, start, end}
		item := series.GetItem(slice)
		if item == nil {
			http.Error(w, "no matching item", 404)
			return
		}
		if contentType == "jpeg" || contentType == "mp4" {
			rd := ReadVideo(*item, slice)
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
				vout, cmd := MakeVideo(rd, item.Width, item.Height)
				_, err := io.Copy(w, vout)
				if err != nil {
					log.Printf("[/series/get-item] error reading video (item %d): %v", item.ID, err)
				}
				cmd.Wait()
			}
		} else if contentType == "json" {
			data := item.Load(slice)
			JsonResponse(w, data)
		} else if contentType == "meta" {
			JsonResponse(w, item)
		}
	})
}
