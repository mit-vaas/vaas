package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

type ParentType string
const (
	OpParent ParentType = "o"
	VideoParent = "v"
)
type Parent struct {
	Type ParentType
	ID int
	Op *Op
}

func (parent Parent) DataType() DataType {
	if parent.Type == OpParent {
		return parent.Op.Type
	} else if parent.Type == VideoParent {
		return VideoType
	}
	panic(fmt.Errorf("bad parent type %v", parent.Type))
}

type Op struct {
	ID int
	Name string
	ParentStrs []string
	Type DataType
	Ext string
	Code string
	Unit int

	Parents []Parent
}

const OpQuery = "SELECT id, name, parents, type, ext, code, unit FROM ops"

func opListHelper(rows Rows) []*Op {
	var ops []*Op
	for rows.Next() {
		var op Op
		var parents string
		rows.Scan(&op.ID, &op.Name, &parents, &op.Type, &op.Ext, &op.Code, &op.Unit)
		op.ParentStrs = strings.Split(parents, ",")
		ops = append(ops, &op)
	}
	return ops
}

func ListOps() []*Op {
	rows := db.Query(OpQuery)
	return opListHelper(rows)
}

func GetOp(id int) *Op {
	rows := db.Query(OpQuery + " WHERE id = ?", id)
	ops := opListHelper(rows)
	if len(ops) == 1 {
		ops[0].Load()
		return ops[0]
	} else {
		return nil
	}
}

func (op *Op) Load() {
	if len(op.Parents) > 0 {
		return
	}
	for _, ps := range op.ParentStrs {
		var parent Parent
		parent.Type = ParentType(ps[0:1])
		if parent.Type == OpParent {
			parent.ID, _ = strconv.Atoi(ps[1:])
			parent.Op = GetOp(parent.ID)
			parent.Op.Load()
		}
		op.Parents = append(op.Parents, parent)
	}
}

func (op *Op) getNode(video Video) *Node {
	node := GetNode(video, fmt.Sprintf("o%d", op.ID))
	if node != nil {
		return node
	} else {
		return &Node{
			ID: -1,
			Video: video,
			Ops: []*Op{op},
			Type: op.Type,
		}
	}
}

// Returns label for this op on the specified clip.
func (op *Op) Test(slices []ClipSlice) []*LabelBuffer {
	// parents[i][j] is the output of parent i on slice j
	log.Printf("[exec op-%v %v] begin op testing", op.Name, slices)
	parents := make([][]*LabelBuffer, len(op.Parents))
	for parentIdx, parent := range op.Parents {
		if parent.Type == OpParent {
			node := parent.Op.getNode(slices[0].Clip.Video)
			for _, labelBuf := range node.Test(slices) {
				/*clipLabel, err := labelBuf.ReadAll(slices[sliceIdx], parent.Op.Type)
				if err != nil {
					log.Printf("[exec op-%v %v] error getting parent %s on slice %v", op.Name, slices, parent.Op.Name, slices[sliceIdx])
					return nil
				}*/
				parents[parentIdx] = append(parents[parentIdx], labelBuf)
			}
		} else if parent.Type == VideoParent {
			for _, slice := range slices {
				rd := ReadVideo(slice, slice.Clip.Width, slice.Clip.Height)
				buf := NewLabelBuffer(VideoType)
				go buf.FromVideoReader(rd)
				parents[parentIdx] = append(parents[parentIdx], buf)
			}
		}
	}

	if op.Ext == "python" {
		return op.testPython(parents, slices)
	}
	return nil
}

func (op *Op) testPython(parents [][]*LabelBuffer, slices []ClipSlice) []*LabelBuffer {
	log.Printf("[exec op-%v %v] launching python script", op.Name, slices)
	template, err := ioutil.ReadFile("tmpl.py")
	if err != nil {
		panic(err)
	}
	script := strings.ReplaceAll(string(template), "[CODE]", op.Code)
	tempFile, err := ioutil.TempFile("", "*.py")
	if err != nil {
		panic(err)
	}
	if _, err := tempFile.Write([]byte(script)); err != nil {
		panic(err)
	}
	if err := tempFile.Close(); err != nil {
		panic(err)
	}
	cmd, stdin, stdout := command(
		fmt.Sprintf("exec-pyop-%s", op.Name), false,
		"/usr/bin/python3", tempFile.Name(),
	)

	go func() {
		writeJSONPacket := func(x interface{}) {
			bytes, err := json.Marshal(x)
			if err != nil {
				panic(err)
			}
			buf := make([]byte, 5)
			binary.BigEndian.PutUint32(buf[0:4], uint32(len(bytes)))
			buf[4] = 'j'
			stdin.Write(buf)
			stdin.Write(bytes)
		}

		writeVideoPacket := func(images []Image) {
			buf := make([]byte, 21)
			l := 16+len(images)*images[0].Width*images[0].Height*3
			binary.BigEndian.PutUint32(buf[0:4], uint32(l))
			buf[4] = 'v'
			binary.BigEndian.PutUint32(buf[5:9], uint32(len(images)))
			binary.BigEndian.PutUint32(buf[9:13], uint32(images[0].Height))
			binary.BigEndian.PutUint32(buf[13:17], uint32(images[0].Width))
			binary.BigEndian.PutUint32(buf[17:21], 3)
			stdin.Write(buf)
			for _, image := range images {
				stdin.Write(image.ToBytes())
			}
		}

		// prepare meta
		var meta struct {
			Type DataType
			Lengths []int
			Count int
			Parents int
		}
		meta.Type = op.Type
		for _, slice := range slices {
			meta.Lengths = append(meta.Lengths, slice.Length())
		}
		meta.Count = len(slices)
		meta.Parents = len(parents)
		writeJSONPacket(meta)

		var mu sync.Mutex
		donech := make(chan error, len(slices))
		for sliceIdx := range slices {
			go func(sliceIdx int) {
				completed := 0
				for completed < slices[sliceIdx].Length() {
					// peek each parent to see how much we can read
					length := -1
					for parentIdx := range parents {
						data, err := parents[parentIdx][sliceIdx].Peek(1)
						if err != nil {
							donech <- err
							return
						}
						if length == -1 || data.Length() < length {
							length = data.Length()
						}
					}

					datas := make([]Data, len(parents))
					for parentIdx := range parents {
						data, err := parents[parentIdx][sliceIdx].Read(length)
						if err != nil {
							donech <- err
							return
						}
						datas[parentIdx] = data
					}

					mu.Lock()
					var job struct {
						SliceIdx int
						Range [2]int
						IsLast bool
					}
					job.SliceIdx = sliceIdx
					job.Range = [2]int{completed, completed+length}
					job.IsLast = job.Range[1] == slices[sliceIdx].Length()
					writeJSONPacket(job)
					for _, data := range datas {
						if data.Type == VideoType {
							writeVideoPacket(data.Images)
						} else {
							writeJSONPacket(data.Get())
						}
					}
					mu.Unlock()
					completed += length
				}

				donech <- nil
			}(sliceIdx)
		}
		for _ = range slices {
			err := <- donech
			if err != nil {
				log.Printf("[exec op-%v %v] error reading from parent: %v", op.Name, slices, err)
				break
			}
		}
		stdin.Close()
	}()

	buffers := make([]*LabelBuffer, len(slices))
	for i := range buffers {
		buffers[i] = NewLabelBuffer(op.Type)
	}

	go func() {
		defer os.Remove(tempFile.Name())
		defer cmd.Wait()
		defer stdin.Close()
		defer stdout.Close()

		dones := make([]bool, len(slices))
		isDone := func() bool {
			for _, done := range dones {
				if !done {
					return false
				}
			}
			return true
		}

		setErr := func(err error) {
			log.Printf("[exec op-%v %v] error during python execution: %v", op.Name, slices, err)
			for i := range buffers {
				buffers[i].Error(err)
			}
		}

		header := make([]byte, 16)
		for !isDone() {
			_, err := io.ReadFull(stdout, header)
			if err != nil {
				setErr(err)
				return
			}
			sliceIdx := int(binary.BigEndian.Uint32(header[0:4]))
			start := int(binary.BigEndian.Uint32(header[4:8]))
			end := int(binary.BigEndian.Uint32(header[8:12]))
			size := int(binary.BigEndian.Uint32(header[12:16]))
			buf := make([]byte, size)
			_, err = io.ReadFull(stdout, buf)
			if err != nil {
				setErr(err)
				return
			}
			data := Data{Type: op.Type}
			if op.Type == DetectionType || op.Type == TrackType {
				if err := json.Unmarshal(buf, &data.Detections); err != nil {
					panic(err)
				}
			} else if op.Type == ClassType {
				if err := json.Unmarshal(buf, &data.Classes); err != nil {
					panic(err)
				}
			} else if op.Type == VideoType {
				nframes := int(binary.BigEndian.Uint32(buf[0:4]))
				height := int(binary.BigEndian.Uint32(buf[4:8]))
				width := int(binary.BigEndian.Uint32(buf[8:12]))
				// TODO: channels buf[12:16]
				chunkSize := width*height*3
				buf = buf[16:]
				for i := 0; i < nframes; i++ {
					data.Images = append(data.Images, ImageFromBytes(width, height, buf[i*chunkSize:(i+1)*chunkSize]))
				}
				//log.Printf("[exec op-%v %v] got the video output", op.Name, slices)
			}
			data = data.EnsureLength(end-start)
			buffers[sliceIdx].Write(data)
			if end >= slices[sliceIdx].Length() {
				dones[sliceIdx] = true
			}
		}
	}()

	return buffers
}

type Node struct {
	ID int
	Query []string
	Video Video
	LabelSetID *int
	Type DataType

	// The selector for a node is based on selector of its first Op.
	Ops []*Op
	LabelSet *LabelSet
}

const NodeQuery = "SELECT n.id, n.query, v.id, v.name, v.ext, n.ls_id, n.type FROM nodes AS n, videos AS v WHERE v.id = n.video_id"

func nodeListHelper(rows Rows) []*Node {
	var nodes []*Node
	for rows.Next() {
		var node Node
		var query string
		rows.Scan(&node.ID, &query, &node.Video.ID, &node.Video.Name, &node.Video.Ext, &node.LabelSetID, &node.Type)
		node.Query = strings.Split(query, ",")
		nodes = append(nodes, &node)
	}
	return nodes
}

func ListNodes() []*Node {
	rows := db.Query(NodeQuery)
	return nodeListHelper(rows)
}

func GetNode(video Video, query string) *Node {
	rows := db.Query(NodeQuery + " AND n.video_id = ? AND n.query = ?", video.ID, query)
	nodes := nodeListHelper(rows)
	if len(nodes) == 1 {
		nodes[0].Load()
		return nodes[0]
	} else {
		return nil
	}
}

func (node *Node) Load() {
	if len(node.Ops) > 0 || node.LabelSet != nil {
		return
	}
	for _, part := range node.Query {
		id, _ := strconv.Atoi(part[1:])
		node.Ops = append(node.Ops, GetOp(id))
	}
	if node.LabelSetID != nil {
		node.LabelSet = GetLabelSet(*node.LabelSetID)
	}
}

// TODO: this should return some kind of LabelWriter so that we can stream the labels back to caller
func (node *Node) Test(slices []ClipSlice) []*LabelBuffer {
	node.Load()
	// (1) if we have the labels in our LabelSet, then just return that
	// (2) TODO: if multiple Ops, then create node for each op and combine the outputs
	//     for now we only support single-op queries...
	// (3) otherwise call Test on our Op
	outputs := make([]*LabelBuffer, len(slices))
	uniqueClips := make(map[int][]int)
	for i, slice := range slices {
		uniqueClips[slice.Clip.ID] = append(uniqueClips[slice.Clip.ID], i)
	}
	var missingSlices []ClipSlice
	var missingIndexes []int // indexes in arg slices
	for _, indexes := range uniqueClips {
		/*ok := func() bool {
			if node.LabelSet == nil {
				return false
			}
			clipLabel := node.LabelSet.GetByClip(slices[indexes[0]].Clip)
			if clipLabel.Label == nil {
				return false
			}
			for _, idx := range indexes {
				outputs[idx] = clipLabel.GetSlice(slices[idx].Start, slices[idx].End)
			}
			return true
		}()
		if !ok {
			missingIndexes = append(missingIndexes, indexes...)
			for _, idx := range indexes {
				missingSlices = append(missingSlices, slices[idx])
			}
		}*/
		for _, idx := range indexes {
			var clipLabel *LabeledClip
			if node.LabelSet != nil {
				clipLabel = node.LabelSet.GetBySlice(slices[idx])
			}
			if clipLabel == nil || clipLabel.Label.Nil() {
				missingSlices = append(missingSlices, slices[idx])
				missingIndexes = append(missingIndexes, idx)
				continue
			}
			outputs[idx] = NewLabelBuffer(clipLabel.Label.Type)
			outputs[idx].Write(clipLabel.Label)
		}
	}
	log.Printf("[exec (%s) %v] missing slices = %v", node.Query, slices, missingSlices)
	if len(missingSlices) == 0 {
		return outputs
	}
	// for now we only handle single ops...
	missingOutputs := node.Ops[0].Test(missingSlices)
	for i := range missingOutputs {
		outputs[missingIndexes[i]] = missingOutputs[i]
	}
	return outputs
}

func init() {
	http.HandleFunc("/ops", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, ListOps())
	})
	http.HandleFunc("/op", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		opID, _ := strconv.Atoi(r.Form.Get("id"))
		op := GetOp(opID)
		if op == nil {
			w.WriteHeader(404)
			return
		}
		if r.Method == "GET" {
			jsonResponse(w, op)
			return
		} else if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		code := r.PostForm.Get("code")
		db.Exec("UPDATE ops SET code = ? WHERE id = ?", code, op.ID)
	})

	http.HandleFunc("/exec/test", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		videoID, _ := strconv.Atoi(r.PostForm.Get("video_id"))
		query := r.PostForm.Get("query")

		video := GetVideo(videoID)
		if video == nil {
			w.WriteHeader(404)
			return
		}
		node := GetNode(*video, query)
		if node == nil {
			w.WriteHeader(404)
			return
		}
		node.Load()
		slice := video.Uniform(VisualizeMaxFrames)
			log.Printf("[exec (%s) %v] beginning test", query, slice)
		labelBuf := node.Test([]ClipSlice{slice})[0]
		if labelBuf == nil {
			w.WriteHeader(404)
			return
		}
		log.Printf("[exec (%s) %v] test: got labelBuf, loading preview", query, slice)
		pc := CreatePreview(slice, node.Type, labelBuf)
		uuid := cache.Add(pc)
		log.Printf("[exec (%s) %v] test: cached preview with %d frames, uuid=%s", query, slice, pc.Slice.Length(), uuid)
		jsonResponse(w, VisualizeResponse{
			PreviewURL: fmt.Sprintf("/cache/preview?id=%s&type=jpeg", uuid),
			URL: fmt.Sprintf("/cache/view?id=%s&type=mp4", uuid),
			Width: slice.Clip.Width,
			Height: slice.Clip.Height,
			UUID: uuid,
		})
	})

	http.HandleFunc("/exec/test2", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		videoID, _ := strconv.Atoi(r.PostForm.Get("video_id"))
		query := r.PostForm.Get("query")

		video := GetVideo(videoID)
		if video == nil {
			w.WriteHeader(404)
			return
		}
		node := GetNode(*video, query)
		if node == nil {
			w.WriteHeader(404)
			return
		}
		node.Load()
		for {
			slice := video.Uniform(VisualizeMaxFrames)
			log.Printf("[exec (%s) %v] beginning test", query, slice)
			labelBuf := node.Test([]ClipSlice{slice})[0]
			if labelBuf == nil {
				w.WriteHeader(404)
				return
			}
			clipLabel, err := labelBuf.ReadAll(slice)
			if err != nil {
				log.Printf("[exec (%s) %v] error reading exec output: %v", query, slice, err)
				w.WriteHeader(400)
				return
			} else if clipLabel.Label.IsEmpty() {
				continue
			}
			log.Printf("[exec (%s) %v] test: got labelBuf, loading preview", query, slice)
			pc := clipLabel.LoadPreview()
			uuid := cache.Add(pc)
			log.Printf("[exec (%s) %v] test: cached preview with %d frames, uuid=%s", query, slice, pc.Slice.Length(), uuid)
			jsonResponse(w, VisualizeResponse{
				PreviewURL: fmt.Sprintf("/cache/preview?id=%s&type=jpeg", uuid),
				URL: fmt.Sprintf("/cache/view?id=%s&type=mp4", uuid),
				Width: slice.Clip.Width,
				Height: slice.Clip.Height,
				UUID: uuid,
			})
			break
		}
	})
}
