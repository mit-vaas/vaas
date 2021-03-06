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
type DBVector struct {
	ID int
	VectorStr string

	Timeline vaas.Timeline
	Vector Vector
}
type DBItem struct {
	vaas.Item
	Percent int
}

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

const SeriesQuery = "SELECT id, timeline_id, name, type, data_type, src_vector, annotate_metadata, node_id FROM series"

func seriesListHelper(rows *Rows) []*DBSeries {
	series := []*DBSeries{}
	for rows.Next() {
		var s DBSeries
		rows.Scan(&s.ID, &s.Timeline.ID, &s.Name, &s.Type, &s.DataType, &s.SrcVectorStr, &s.AnnotateMetadata, &s.NodeID)
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
		series := GetSeries(id)
		if series == nil {
			panic(fmt.Errorf("no series with id=%d", id))
		}
		vector = append(vector, series)
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

func (v Vector) Pretty() string {
	var parts []string
	for _, series := range v {
		parts = append(parts, series.Name)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func VectorFromList(l []vaas.Series) Vector {
	var vector Vector
	for _, series := range l {
		vector = append(vector, GetSeries(series.ID))
	}
	return vector
}

const VectorQuery = "SELECT id, timeline_id, vector FROM vectors"

func vectorListHelper(rows *Rows) []DBVector {
	var vectors []DBVector
	for rows.Next() {
		var v DBVector
		rows.Scan(&v.ID, &v.Timeline.ID, &v.VectorStr)
		vectors = append(vectors, v)
	}
	for i := range vectors {
		vectors[i].Timeline = GetTimeline(vectors[i].Timeline.ID).Timeline
		vectors[i].Vector = ParseVector(vectors[i].VectorStr)
	}
	return vectors
}

func ListVectors() []DBVector {
	rows := db.Query(VectorQuery)
	return vectorListHelper(rows)
}

func GetVector(id int) *DBVector {
	rows := db.Query(VectorQuery + " WHERE id = ?", id)
	vectors := vectorListHelper(rows)
	if len(vectors) == 1 {
		return &vectors[0]
	} else {
		return nil
	}
}

func (timeline DBTimeline) ListVectors() []DBVector {
	rows := db.Query(VectorQuery + " WHERE timeline_id = ?", timeline.ID)
	return vectorListHelper(rows)
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

type SliceSampler []vaas.Slice
func SliceSamplerFromSegments(segments []DBSegment) SliceSampler {
	var sampler SliceSampler
	for _, segment := range segments {
		sampler = append(sampler, segment.ToSlice())
	}
	return sampler
}
func (slices SliceSampler) Uniform(unit int) vaas.Slice {
	// select a large slice
	slice := func() vaas.Slice {
		if unit == 0 {
			return slices[rand.Intn(len(slices))]
		}
		weights := make([]int, len(slices))
		sum := 0
		for i, slice := range slices {
			weights[i] = (slice.Length()+unit-1) / unit
			if weights[i] < 0 {
				weights[i] = 0
			}
			sum += weights[i]
		}
		if sum == 0 {
			panic(fmt.Errorf("no possible slice"))
		}
		r := rand.Intn(sum)
		for i, slice := range slices {
			r -= weights[i]
			if r <= 0 {
				return slice
			}
		}
		return slices[len(slices)-1]
	}()

	// select small slice from the large slice
	var start, end int
	if unit == 0 {
		start = slice.Start
		end = slice.End
	} else {
		// we select the small slice so that, if large slices correspond to entire segments,
		// then the small slices start/end at multiples of unit
		// but if the large slices are smaller then there is no guarantee
		idx := rand.Intn((slice.Length()+unit-1)/unit)
		start = slice.Start + idx*unit
		end = slice.Start + (idx+1)*unit
		if end > slice.End {
			end = slice.End
		}
	}

	return vaas.Slice{slice.Segment, start, end}
}

func (timeline DBTimeline) Uniform(unit int) vaas.Slice {
	segments := timeline.ListSegments()
	return SliceSamplerFromSegments(segments).Uniform(unit)
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
	db.Exec("UPDATE vnodes SET series_id = NULL WHERE series_id = ?", series.ID)
}

func (series DBSeries) Next(nframes int) vaas.Slice {
	return DBTimeline{series.Timeline}.Uniform(nframes)
}

const ItemQuery = "SELECT id, segment_id, series_id, start, end, format, width, height, freq, percent FROM items"

func itemListHelper(rows *Rows) []DBItem {
	var items []DBItem
	for rows.Next() {
		var item DBItem
		rows.Scan(&item.ID, &item.Slice.Segment.ID, &item.Series.ID, &item.Slice.Start, &item.Slice.End, &item.Format, &item.Width, &item.Height, &item.Freq, &item.Percent)
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
	opts := vaas.ExecOptions{
		PersistVideo: true,
		Outputs: [][]vaas.Parent{{vaas.Parent{
			Type: vaas.NodeParent,
			NodeID: series.Node.ID,
		}}},
	}
	vector := VectorFromList(series.SrcVector)
	bufs, err := query.RunBuffer(vector, slice, opts)
	if err != nil {
		return nil, err
	}
	return bufs[0][0], nil
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

func NewTimeline(name string) *DBTimeline {
	res := db.Exec("INSERT INTO timelines (name) VALUES (?)", name)
	return GetTimeline(res.LastInsertId())
}

func NewSeries(timelineID int, name string, dataType vaas.DataType) *DBSeries {
	res := db.Exec("INSERT INTO series (timeline_id, name, type, data_type) VALUES (?, ?, 'data', ?)", timelineID, name, dataType)
	return GetSeries(res.LastInsertId())
}

func init() {
	http.HandleFunc("/datasets", func(w http.ResponseWriter, r *http.Request) {
		vaas.JsonResponse(w, ListSeriesByType("data"))
	})

	http.HandleFunc("/series", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			vaas.JsonResponse(w, ListSeries())
			return
		} else if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		r.ParseForm()
		timelineID := vaas.ParseInt(r.PostForm.Get("timeline_id"))
		name := r.PostForm.Get("name")
		dataType := r.PostForm.Get("data_type")
		series := NewSeries(timelineID, name, vaas.DataType(dataType))
		vaas.JsonResponse(w, series)
	})

	http.HandleFunc("/timelines", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			type JsonTimeline struct {
				DBTimeline
				NumDataSeries int
				NumLabelSeries int
				NumOutputSeries int
				CanDelete bool
			}
			timelines := []JsonTimeline{}
			for _, timeline := range ListTimelines() {
				jt := JsonTimeline{}
				jt.DBTimeline = timeline
				db.QueryRow(
					`SELECT IFNULL(SUM(type == 'data'), 0), IFNULL(SUM(type == 'labels'), 0), IFNULL(SUM(type == 'outputs'), 0)
					FROM series WHERE timeline_id = ?`,
					timeline.ID,
				).Scan(&jt.NumDataSeries, &jt.NumLabelSeries, &jt.NumOutputSeries)
				jt.CanDelete = jt.NumDataSeries == 0 && jt.NumLabelSeries == 0 && jt.NumOutputSeries == 0
				timelines = append(timelines, jt)
			}
			vaas.JsonResponse(w, timelines)
			return
		} else if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		// add new timeline
		r.ParseForm()
		name := r.PostForm.Get("name")
		NewTimeline(name)
	})

	http.HandleFunc("/timelines/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		r.ParseForm()
		timelineID := vaas.ParseInt(r.PostForm.Get("timeline_id"))
		var seriesCount int
		db.QueryRow("SELECT COUNT(*) FROM series WHERE timeline_id = ?", timelineID).Scan(&seriesCount)
		if seriesCount > 0 {
			http.Error(w, "delete all series in the timeline first", 400)
			return
		}
		db.Exec("DELETE FROM timelines WHERE id = ?", timelineID)
	})

	http.HandleFunc("/timeline/series", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		timelineID := vaas.ParseInt(r.Form.Get("timeline_id"))
		getSeriesByType := func(t string) []*DBSeries {
			rows := db.Query(SeriesQuery + " WHERE timeline_id = ? AND type = ?", timelineID, t)
			return seriesListHelper(rows)
		}
		var response struct {
			DataSeries []*DBSeries
			LabelSeries []*DBSeries
			OutputSeries []*DBSeries
		}
		response.DataSeries = getSeriesByType("data")
		response.LabelSeries = getSeriesByType("labels")
		response.OutputSeries = getSeriesByType("outputs")
		vaas.JsonResponse(w, response)
	})

	http.HandleFunc("/timeline/vectors", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		timelineID := vaas.ParseInt(r.Form.Get("timeline_id"))
		timeline := GetTimeline(timelineID)
		if timeline == nil {
			http.Error(w, "no such timeline", 404)
			return
		}

		if r.Method == "GET" {
			vaas.JsonResponse(w, timeline.ListVectors())
			return
		} else if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		vectorStr := r.PostForm.Get("series_ids")
		db.Exec("INSERT INTO vectors (timeline_id, vector) VALUES (?, ?)", timeline.ID, vectorStr)
	})

	http.HandleFunc("/series/items", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		seriesID := vaas.ParseInt(r.Form.Get("series_id"))
		series := GetSeries(seriesID)
		if series == nil {
			http.Error(w, "no such series", 404)
			return
		}
		vaas.JsonResponse(w, series.ListItems())
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

	http.HandleFunc("/series/delete-item", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		r.ParseForm()
		itemID := vaas.ParseInt(r.PostForm.Get("item_id"))
		item := GetItem(itemID)
		if item == nil {
			w.WriteHeader(404)
			return
		}
		item.Delete()
	})

	http.HandleFunc("/series/update", func(w http.ResponseWriter, r *http.Request) {
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
		if r.PostForm["annotate_metadata"] != nil {
			db.Exec("UPDATE series SET annotate_metadata = ? WHERE id = ?", r.PostForm.Get("annotate_metadata"), series.ID)
		}
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
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

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

	http.HandleFunc("/vectors", func(w http.ResponseWriter, r *http.Request) {
		vaas.JsonResponse(w, ListVectors())
	})

	http.HandleFunc("/vectors/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		r.ParseForm()
		vectorID := vaas.ParseInt(r.PostForm.Get("vector_id"))
		db.Exec("DELETE FROM vectors WHERE id = ?", vectorID)
	})

	http.HandleFunc("/ensure-output-series", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		r.ParseForm()
		nodeID := vaas.ParseInt(r.PostForm.Get("node_id"))
		vector := ParseVector(r.PostForm.Get("vector"))
		node := GetNode(nodeID)
		if node == nil {
			http.Error(w, "no such node", 404)
			return
		}
		vn := GetOrCreateVNode(node, vector)
		vn.EnsureSeries()
	})
}
