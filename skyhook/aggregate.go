package skyhook

import (
	gomapinfer "github.com/mitroadmaps/gomapinfer/common"

	"fmt"
	"log"
	"net/http"
)

type LabeledSlice struct {
	Slice Slice
	Background *Series
	Data Data
}

// specifier/reference for where we can get one or more LabeledSlices
type LabeledSliceRef struct {
	Background Series

	Series *Series // reference an entire series
	Item *Item // reference a specific item

	// the remaining references require Slice and Vector
	Slice Slice
	Vector string

	Node *Node // reference the output of a node on Slice
	DataType DataType
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
			err := LabeledSliceRef{Item: &item}.Resolve(f)
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
		slice := Slice{*segment, r.Slice.Start, r.Slice.End}
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
		item := vn.Series.GetItem(slice)
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
		slice := Slice{*segment, r.Slice.Start, r.Slice.End}
		data := DecodeData(r.DataType, []byte(r.Data)).EnsureLength(slice.Length())
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
		if err := ParseJsonRequest(w, r, &request); err != nil {
			return
		}

		var im *Image

		f := func(l LabeledSlice) error {
			if im == nil {
				slice := Slice{l.Slice.Segment, l.Slice.Start, l.Slice.Start+1}
				bgItem := l.Background.GetItem(slice)
				rd := ReadVideo(*bgItem,  slice, ReadVideoOptions{})
				x, err := rd.Read()
				rd.Close()
				if err != nil {
					return err
				}
				im = &x
			}

			getCenter := func(d Detection) [2]int {
				return [2]int{
					(d.Left+d.Right)/2,
					(d.Top+d.Bottom)/2,
				}
			}

			if l.Data.Type() == DetectionType {
				detections := l.Data.(DetectionData)
				for _, dlist := range detections {
					for _, d := range dlist {
						center := getCenter(d)
						im.FillRectangle(center[0]-1, center[1]-1, center[0]+1, center[1]+1, [3]uint8{255, 0, 0})
					}
				}
			} else if l.Data.Type() == TrackType {
				detections := l.Data.(TrackData)
				for _, track := range DetectionsToTracks(detections) {
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

		id := cache.Add([]Image{*im})
		JsonResponse(w, AggregateResponse{
			fmt.Sprintf("/cache/view?id=%s&type=jpeg", id),
		})
	})
}
