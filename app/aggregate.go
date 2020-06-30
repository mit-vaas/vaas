package app

import (
	"../vaas"
	gomapinfer "github.com/mitroadmaps/gomapinfer/common"

	"fmt"
	"log"
	"net/http"
)

type LabeledSlice struct {
	Slice vaas.Slice
	Background *DBSeries
	Data vaas.Data
}

// specifier/reference for where we can get one or more LabeledSlices
type LabeledSliceRef struct {
	Background vaas.Series

	Series *vaas.Series // reference an entire series
	Item *vaas.Item // reference a specific item

	// the remaining references require Slice and Vector
	Slice vaas.Slice
	Vector string

	Node *vaas.Node // reference the output of a node on Slice
	DataType vaas.DataType
	Data string // include the label data directly
}

// yield LabeledSlices from this ref until we exhaust them or encounter an error
func (r LabeledSliceRef) Resolve(f func(l LabeledSlice) error) error {
	background := GetSeries(r.Background.ID)

	if r.Series != nil {
		series := GetSeries(r.Series.ID)
		if series == nil {
			return fmt.Errorf("no such series")
		}
		for _, item := range series.ListItems() {
			item_ := item.Item
			err := LabeledSliceRef{Item: &item_}.Resolve(f)
			if err != nil {
				return err
			}
		}
		return nil
	} else if r.Item != nil {
		item := GetItem(r.Item.ID)
		if item == nil {
			return fmt.Errorf("no such item")
		}
		data, err := item.Load(item.Slice).Reader().Read(item.Slice.Length())
		if err != nil {
			return fmt.Errorf("error reading item %d: %v", item.ID, err)
		}
		return f(LabeledSlice{
			Slice: item.Slice,
			Background: background,
			Data: data,
		})
	} else if r.Node != nil {
		segment := GetSegment(r.Slice.Segment.ID)
		slice := vaas.Slice{segment.Segment, r.Slice.Start, r.Slice.End}
		vector := ParseVector(r.Vector)
		node := GetNode(r.Node.ID)
		if node == nil {
			return fmt.Errorf("no such node")
		}
		vn := GetVNode(node, vector)
		if vn == nil {
			return fmt.Errorf("no saved vnode")
		}
		vn.Load()
		item := DBSeries{Series: *vn.Series}.GetItem(slice)
		if item == nil {
			return fmt.Errorf("no saved item")
		}
		data, err := item.Load(slice).Reader().Read(slice.Length())
		if err != nil {
			return fmt.Errorf("error reading item %d: %v", item.ID, err)
		}
		return f(LabeledSlice{
			Slice: slice,
			Background: background,
			Data: data,
		})
	} else if len(r.Data) > 0 {
		segment := GetSegment(r.Slice.Segment.ID)
		slice := vaas.Slice{segment.Segment, r.Slice.Start, r.Slice.End}
		data := vaas.DecodeData(r.DataType, []byte(r.Data)).EnsureLength(slice.Length())
		return f(LabeledSlice{
			Slice: slice,
			Background: background,
			Data: data,
		})
	}
	panic(fmt.Errorf("bad LabeledSliceSpec"))
}

func init() {
	type AggregateResponse struct {
		URL string
	}

	http.HandleFunc("/aggregates/scatter", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		var request []LabeledSliceRef
		if err := vaas.ParseJsonRequest(w, r, &request); err != nil {
			return
		}

		var im *vaas.Image

		f := func(l LabeledSlice) error {
			if im == nil {
				slice := vaas.Slice{l.Slice.Segment, l.Slice.Start, l.Slice.Start+1}
				bgItem := l.Background.GetItem(slice)
				rd := vaas.ReadVideo(bgItem.Item,  slice, vaas.ReadVideoOptions{})
				x, err := rd.Read()
				rd.Close()
				if err != nil {
					return err
				}
				im = &x
			}

			getCenter := func(d vaas.Detection) [2]int {
				return [2]int{
					(d.Left+d.Right)/2,
					(d.Top+d.Bottom)/2,
				}
			}

			if l.Data.Type() == vaas.DetectionType {
				detections := l.Data.(vaas.DetectionData)
				for _, dlist := range detections {
					for _, d := range dlist {
						center := getCenter(d)
						im.FillRectangle(center[0]-1, center[1]-1, center[0]+1, center[1]+1, [3]uint8{255, 0, 0})
					}
				}
			} else if l.Data.Type() == vaas.TrackType {
				detections := l.Data.(vaas.TrackData)
				for _, track := range vaas.DetectionsToTracks(detections) {
					for i := 1; i < len(track); i++ {
						c1 := getCenter(track[i-1].Detection)
						c2 := getCenter(track[i].Detection)
						for _, p := range gomapinfer.DrawLineOnCells(c1[0], c1[1], c2[0], c2[1], im.Width, im.Height) {
							im.FillRectangle(p[0]-1, p[1]-1, p[0]+1, p[1]+1, [3]uint8{255, 0, 0})
						}
					}
				}
			}

			return nil
		}

		for _, ref := range request {
			err := ref.Resolve(f)
			if err != nil {
				log.Printf("[/aggregates/scatter] failed to create scatter image: %v", err)
				http.Error(w, fmt.Sprintf("failed to create scatter image: %v", err), 400)
				return
			}
		}

		if im == nil {
			http.Error(w, fmt.Sprintf("no labels processed"), 400)
			return
		}

		id := cache.Add([]vaas.Image{*im})
		vaas.JsonResponse(w, AggregateResponse{
			fmt.Sprintf("/cache/view?id=%s&type=jpeg", id),
		})
	})
}
