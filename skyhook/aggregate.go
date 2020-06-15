package skyhook

import (
	gomapinfer "github.com/mitroadmaps/gomapinfer/common"

	"fmt"
	"log"
	"net/http"
)

type LabeledSlice struct {
	Slice ClipSlice
	Data Data
}

// specifier/reference for where we can get one or more LabeledSlices
type LabeledSliceRef struct {
	LabelSet *LabelSet // reference an entire label set
	Label *Label // reference a specific label

	// the remaining references require Slice
	Slice ClipSlice

	Node *Node // reference the output of a node on Slice
	Data *Data // include the label data directly
}

// yield LabeledSlices from this ref until we exhaust them or encounter an error
func (r LabeledSliceRef) Resolve(f func(l LabeledSlice) error) error {
	if r.LabelSet != nil {
		ls := GetLabelSet(r.LabelSet.ID)
		if ls == nil {
			return fmt.Errorf("no such label set")
		}
		for _, label := range ls.ListLabels() {
			err := LabeledSliceRef{Label: &label}.Resolve(f)
			if err != nil {
				return err
			}
		}
		return nil
	} else if r.Label != nil {
		label := GetLabel(r.Label.ID)
		if label == nil {
			return fmt.Errorf("no such label")
		}
		data, err := label.Load(label.Slice).Reader().Read(label.Slice.Length())
		if err != nil {
			return fmt.Errorf("error reading label %d: %v", label.ID, err)
		}
		return f(LabeledSlice{label.Slice, data})
	} else if r.Node != nil {
		clip := GetClip(r.Slice.Clip.ID)
		slice := ClipSlice{*clip, r.Slice.Start, r.Slice.End}
		node := GetNode(r.Node.ID)
		if node == nil {
			return fmt.Errorf("no such node")
		}
		vn := GetVNode(node, clip.Video)
		if vn == nil {
			return fmt.Errorf("no saved vnode")
		}
		vn.Load()
		label := vn.LabelSet.GetBySlice(slice)
		if label == nil {
			return fmt.Errorf("no saved label")
		}
		data, err := label.Load(slice).Reader().Read(slice.Length())
		if err != nil {
			return fmt.Errorf("error reading label %d: %v", label.ID, err)
		}
		return f(LabeledSlice{slice, data})
	} else if r.Data != nil {
		clip := GetClip(r.Slice.Clip.ID)
		slice := ClipSlice{*clip, r.Slice.Start, r.Slice.End}
		data := *r.Data
		data = data.EnsureLength(slice.Length())
		return f(LabeledSlice{slice, data})
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
		if err := JsonRequest(w, r, &request); err != nil {
			return
		}

		var im *Image

		f := func(l LabeledSlice) error {
			if im == nil {
				slice := ClipSlice{l.Slice.Clip, l.Slice.Start, l.Slice.Start+1}
				rd := ReadVideo(slice, slice.Clip.Width, slice.Clip.Height)
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

			if l.Data.Type == DetectionType {
				for _, dlist := range l.Data.Detections {
					for _, d := range dlist {
						center := getCenter(d)
						im.FillRectangle(center[0]-1, center[1]-1, center[0]+1, center[1]+1, [3]uint8{255, 0, 0})
					}
				}
			} else if l.Data.Type == TrackType {
				for _, track := range DetectionsToTracks(l.Data.Detections) {
					for i := 1; i < len(track); i++ {
						c1 := getCenter(track[i-1])
						c2 := getCenter(track[i])
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
