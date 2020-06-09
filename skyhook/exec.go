package skyhook

import (
	"encoding/binary"
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
	NodeParent ParentType = "o"
	VideoParent = "v"
)
type Parent struct {
	Type ParentType
	ID int
	Node *Node
}

func (parent Parent) DataType() DataType {
	if parent.Type == NodeParent {
		return parent.Node.Type
	} else if parent.Type == VideoParent {
		return VideoType
	}
	panic(fmt.Errorf("bad parent type %v", parent.Type))
}

type Node struct {
	ID int
	Name string
	ParentSpecs []Parent // parents without *Node filled in
	Type DataType
	Ext string
	Code string

	Parents []Parent
}

const NodeQuery = "SELECT id, name, parents, type, ext, code FROM nodes"

func nodeListHelper(rows *Rows) []*Node {
	var nodes []*Node
	for rows.Next() {
		var node Node
		var parents string
		rows.Scan(&node.ID, &node.Name, &parents, &node.Type, &node.Ext, &node.Code)
		for _, ps := range strings.Split(parents, ",") {
			var parent Parent
			parent.Type = ParentType(ps[0:1])
			if parent.Type == NodeParent {
				parent.ID, _ = strconv.Atoi(ps[1:])
			}
			node.ParentSpecs = append(node.ParentSpecs, parent)
		}
		nodes = append(nodes, &node)
	}
	return nodes
}

func ListNodes() []*Node {
	rows := db.Query(NodeQuery)
	return nodeListHelper(rows)
}

func GetNode(id int) *Node {
	rows := db.Query(NodeQuery + " WHERE id = ?", id)
	nodes := nodeListHelper(rows)
	if len(nodes) == 1 {
		nodes[0].Load()
		return nodes[0]
	} else {
		return nil
	}
}

func (node *Node) Load() {
	if len(node.Parents) > 0 {
		return
	}
	for _, parent := range node.ParentSpecs {
		if parent.Type == NodeParent {
			parent.Node = GetNode(parent.ID)
			parent.Node.Load()
		}
		node.Parents = append(node.Parents, parent)
	}
}

func (node *Node) ListVNodes() []*VNode {
	return ListVNodesWithNode(node)
}

// Returns label for this node on the specified clip.
func (node *Node) Test(slices []ClipSlice) []*LabelBuffer {
	// parents[i][j] is the output of parent i on slice j
	log.Printf("[exec node-%v %v] begin node testing", node.Name, slices)
	parents := make([][]*LabelBuffer, len(node.Parents))
	for parentIdx, parent := range node.Parents {
		if parent.Type == NodeParent {
			parentVN := GetVNode(parent.Node, slices[0].Clip.Video)
			for _, labelBuf := range parentVN.Test(slices) {
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

	if node.Ext == "python" {
		return node.testPython(parents, slices)
	} else if node.Ext == "model" {
		return node.testModel(parents, slices)
	}
	return nil
}

func (node *Node) testPython(parents [][]*LabelBuffer, slices []ClipSlice) []*LabelBuffer {
	log.Printf("[exec node-%v %v] launching python script", node.Name, slices)
	template, err := ioutil.ReadFile("tmpl.py")
	if err != nil {
		panic(err)
	}
	script := strings.ReplaceAll(string(template), "[CODE]", node.Code)
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
	cmd, stdin, stdout := Command(
		fmt.Sprintf("exec-python-%s", node.Name), CommandOptions{},
		"/usr/bin/python3", tempFile.Name(),
	)

	go func() {
		writeJSONPacket := func(x interface{}) {
			bytes := JsonMarshal(x)
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
		meta.Type = node.Type
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
						data, err := parents[parentIdx][sliceIdx].Peek(completed, 1)
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
						data, err := parents[parentIdx][sliceIdx].Read(completed, length)
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
				log.Printf("[exec node-%v %v] error reading from parent: %v", node.Name, slices, err)
				break
			}
		}
		stdin.Close()
	}()

	buffers := make([]*LabelBuffer, len(slices))
	for i := range buffers {
		buffers[i] = NewLabelBuffer(node.Type)
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
			log.Printf("[exec node-%v %v] error during python execution: %v", node.Name, slices, err)
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
			data := Data{Type: node.Type}
			if node.Type == DetectionType || node.Type == TrackType {
				JsonUnmarshal(buf, &data.Detections)
			} else if node.Type == ClassType {
				JsonUnmarshal(buf, &data.Classes)
			} else if node.Type == VideoType {
				nframes := int(binary.BigEndian.Uint32(buf[0:4]))
				height := int(binary.BigEndian.Uint32(buf[4:8]))
				width := int(binary.BigEndian.Uint32(buf[8:12]))
				// TODO: channels buf[12:16]
				chunkSize := width*height*3
				buf = buf[16:]
				for i := 0; i < nframes; i++ {
					data.Images = append(data.Images, ImageFromBytes(width, height, buf[i*chunkSize:(i+1)*chunkSize]))
				}
				//log.Printf("[exec node-%v %v] got the video output", node.Name, slices)
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

type VNode struct {
	ID int
	NodeID int
	Video Video
	LabelSetID *int

	Node *Node
	LabelSet *LabelSet
}

const VNodeQuery = "SELECT vn.id, vn.node_id, v.id, v.name, v.ext, vn.ls_id FROM vnodes AS vn, videos AS v WHERE v.id = vn.video_id"

func vnodeListHelper(rows *Rows) []*VNode {
	var vnodes []*VNode
	for rows.Next() {
		var vn VNode
		rows.Scan(&vn.ID, &vn.NodeID, &vn.Video.ID, &vn.Video.Name, &vn.Video.Ext, &vn.LabelSetID)
		vnodes = append(vnodes, &vn)
	}
	return vnodes
}

func GetVNode(node *Node, video Video) *VNode {
	rows := db.Query(VNodeQuery + " AND vn.video_id = ? AND vn.node_id = ?", video.ID, node.ID)
	nodes := vnodeListHelper(rows)
	if len(nodes) == 1 {
		nodes[0].Load()
		return nodes[0]
	} else {
		return nil
	}
}

func GetOrCreateVNode(node *Node, video Video) *VNode {
	vn := GetVNode(node, video)
	if vn != nil {
		return vn
	}
	db.Exec("INSERT INTO vnodes (node_id, video_id) VALUES (?, ?)", node.ID, video.ID)
	return GetVNode(node, video)
}

func ListVNodesWithNode(node *Node) []*VNode {
	rows := db.Query(VNodeQuery + " AND vn.node_id = ?", node.ID)
	return vnodeListHelper(rows)
}

func (vn *VNode) Load() {
	if vn.Node != nil || vn.LabelSet != nil {
		return
	}
	vn.Node = GetNode(vn.NodeID)
	if vn.LabelSetID != nil {
		vn.LabelSet = GetLabelSet(*vn.LabelSetID)
	}
}

func (vn *VNode) Test(slices []ClipSlice) []*LabelBuffer {
	vn.Load()
	// if we have the labels in our LabelSet, then just return that
	// otherwise call Test on our Node
	outputs := make([]*LabelBuffer, len(slices))
	uniqueClips := make(map[int][]int)
	for i, slice := range slices {
		uniqueClips[slice.Clip.ID] = append(uniqueClips[slice.Clip.ID], i)
	}
	var missingSlices []ClipSlice
	var missingIndexes []int // indexes in arg slices
	for _, indexes := range uniqueClips {
		for _, idx := range indexes {
			if vn.LabelSet != nil {
				label := vn.LabelSet.GetBySlice(slices[idx])
				if label != nil {
					outputs[idx] = label.Load(slices[idx])
					continue
				}
			}
			missingSlices = append(missingSlices, slices[idx])
			missingIndexes = append(missingIndexes, idx)
		}
	}
	log.Printf("[exec (%s) %v] missing slices = %v", vn.Node.Name, slices, missingSlices)
	if len(missingSlices) == 0 {
		return outputs
	}

	// if no label set, create one to store the exec outputs
	if vn.LabelSet == nil {
		db.Transaction(func(tx Tx) {
			var lsID *int
			tx.QueryRow("SELECT ls_id FROM vnodes WHERE id = ?", vn.ID).Scan(&lsID)
			if lsID != nil {
				vn.LabelSetID = lsID
				return
			}
			name := fmt.Sprintf("exec-%v-%d", vn.Node.Name, vn.ID)
			res := tx.Exec(
				"INSERT INTO label_sets (name, unit, src_video, video_id, label_type) VALUES (?, ?, ?, ?, ?)",
				name, 1, vn.Video.ID, vn.Video.ID, vn.Node.Type,
			)
			vn.LabelSetID = new(int)
			*vn.LabelSetID = res.LastInsertId()
			tx.Exec("UPDATE vnodes SET ls_id = ? WHERE id = ?", vn.LabelSetID, vn.ID)
		})
		vn.LabelSet = GetLabelSet(*vn.LabelSetID)
	}

	missingOutputs := vn.Node.Test(missingSlices)
	for i := range missingOutputs {
		outputs[missingIndexes[i]] = missingOutputs[i]
	}

	// write the outputs
	go func() {
		for i := range missingOutputs {
			data, err := missingOutputs[i].ReadFull(missingSlices[i].Length())
			if err != nil {
				continue
			}
			vn.LabelSet.AddLabel(missingSlices[i], data)
		}
	}()

	return outputs
}

// clear saved labels at a node
func (vn *VNode) Clear() {
	vn.Load()
	if vn.LabelSet != nil {
		log.Printf("[exec (%s)] clearing label set %d", vn.Node.Name, vn.LabelSet.ID)
		vn.LabelSet.Clear()
	}
}

func init() {
	http.HandleFunc("/nodes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			JsonResponse(w, ListNodes())
			return
		} else if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		r.ParseForm()
		name := r.PostForm.Get("name")
		parents := r.PostForm.Get("parents")
		t := r.PostForm.Get("type")
		ext := r.PostForm.Get("ext")
		db.Exec(
			"INSERT INTO nodes (name, parents, type, ext, code) VALUES (?, ?, ?, ?, '')",
			name, parents, t, ext,
		)
	})
	http.HandleFunc("/node", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		nodeID, _ := strconv.Atoi(r.Form.Get("id"))
		node := GetNode(nodeID)
		if node == nil {
			w.WriteHeader(404)
			return
		}
		if r.Method == "GET" {
			JsonResponse(w, node)
			return
		} else if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}

		code := r.PostForm.Get("code")
		db.Exec("UPDATE nodes SET code = ? WHERE id = ?", code, node.ID)

		// delete all saved labels at the vnodes for this node
		// and recursively delete for vnodes that depend on that vnode
		affectedNodes := map[int]*Node{node.ID: node}
		queue := []*Node{node}
		allNodes := ListNodes()
		for len(queue) > 0 {
			head := queue[0]
			queue = queue[1:]
			// find all children of head
			for _, node := range allNodes {
				if affectedNodes[node.ID] != nil {
					continue
				}
				isChild := false
				for _, parent := range node.ParentSpecs {
					if parent.Type != NodeParent {
						continue
					}
					if parent.ID != head.ID {
						continue
					}
					isChild = true
				}
				if !isChild {
					continue
				}

				affectedNodes[node.ID] = node
				queue = append(queue, node)
			}
		}

		for _, node := range affectedNodes {
			for _, vn := range node.ListVNodes() {
				vn.Clear()
			}
		}
	})

	http.HandleFunc("/exec/test", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		videoID, _ := strconv.Atoi(r.PostForm.Get("video_id"))
		nodeID, _ := strconv.Atoi(r.PostForm.Get("node_id"))

		node := GetNode(nodeID)
		if node == nil {
			w.WriteHeader(404)
			return
		}
		video := GetVideo(videoID)
		if video == nil {
			w.WriteHeader(404)
			return
		}
		vn := GetOrCreateVNode(node, *video)
		vn.Load()
		slice := video.Uniform(VisualizeMaxFrames)
		log.Printf("[exec (%s) %v] beginning test", node.Name, slice)
		labelBuf := vn.Test([]ClipSlice{slice})[0]
		if labelBuf == nil {
			w.WriteHeader(404)
			return
		}
		log.Printf("[exec (%s) %v] test: got labelBuf, loading preview", node.Name, slice)
		pc := CreatePreview(slice, labelBuf)
		uuid := cache.Add(pc)
		log.Printf("[exec (%s) %v] test: cached preview with %d frames, uuid=%s", node.Name, slice, pc.Slice.Length(), uuid)
		JsonResponse(w, VisualizeResponse{
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
		nodeID, _ := strconv.Atoi(r.PostForm.Get("node_id"))

		node := GetNode(nodeID)
		if node == nil {
			w.WriteHeader(404)
			return
		}
		video := GetVideo(videoID)
		if video == nil {
			w.WriteHeader(404)
			return
		}
		vn := GetOrCreateVNode(node, *video)
		vn.Load()
		for {
			slice := video.Uniform(VisualizeMaxFrames)
			log.Printf("[exec (%s) %v] beginning test", node.Name, slice)
			labelBuf := vn.Test([]ClipSlice{slice})[0]
			if labelBuf == nil {
				w.WriteHeader(404)
				return
			}
			data, err := labelBuf.ReadFull(slice.Length())
			if err != nil {
				log.Printf("[exec (%s) %v] error reading exec output: %v", node.Name, slice, err)
				w.WriteHeader(400)
				return
			} else if data.IsEmpty() {
				continue
			}
			log.Printf("[exec (%s) %v] test: got labelBuf, loading preview", node.Name, slice)
			nbuf := NewLabelBuffer(data.Type)
			nbuf.Write(data)
			pc := CreatePreview(slice, nbuf)
			uuid := cache.Add(pc)
			log.Printf("[exec (%s) %v] test: cached preview with %d frames, uuid=%s", node.Name, slice, pc.Slice.Length(), uuid)
			JsonResponse(w, VisualizeResponse{
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
