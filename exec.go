package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type SelType string
const (
	FrameSel SelType = "frame"
	TrackSel = "track"
)

type Selector struct {
	SelType SelType
	Frames int
}

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
	} else {
		// TODO: video?
		return ""
	}
}

type Op struct {
	ID int
	Name string
	ParentStrs []string
	Type DataType
	Ext string
	Code string
	Sel Selector

	Parents []Parent
}

const OpQuery = "SELECT id, name, parents, type, ext, code, sel_type, sel_frames FROM ops"

func opListHelper(rows Rows) []*Op {
	var ops []*Op
	for rows.Next() {
		var op Op
		var parents string
		rows.Scan(&op.ID, &op.Name, &parents, &op.Type, &op.Ext, &op.Code, &op.Sel.SelType, &op.Sel.Frames)
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
		parent.ID, _ = strconv.Atoi(ps[1:])
		if parent.Type == OpParent {
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
func (op *Op) Test(slices []ClipSlice) []*LabeledClip {
	// parents[i][j] is the output of parent i on slice j
	var parents [][]*LabeledClip
	for _, parent := range op.Parents {
		if parent.Type == OpParent {
			node := parent.Op.getNode(slices[0].Clip.Video)
			parents = append(parents, node.Test(slices))
		} else {
			// ...?
		}
	}

	// inputs[i][j] is the jth input for slice i
	// the input is a tuple: input[k] is contribution from parent k
	inputs := make([][][]interface{}, len(slices))
	if op.Sel.SelType == TrackSel {
		// for track sel, the first parent must be track type
		// we use those tracks as the basis, and re-map all the
		//   other parents onto those tracks
		for i := range slices {
			detections := parents[0][i].Label.([][]Detection)
			tracks := DetectionsToTracks(detections)
			for _, track := range tracks {
				var input []interface{}
				input = append(input, track)
				// TODO: remap parents[1..n][i] to the track...
				inputs[i] = append(inputs[i], input)
			}
		}
	}

	if op.Ext == "python" {
		return op.testPython(inputs, slices)
	}
	return nil
}

func (op *Op) testPython(inputs [][][]interface{}, slices []ClipSlice) []*LabeledClip {
	template, err := ioutil.ReadFile("tmpl.py")
	if err != nil {
		panic(err)
	}
	script := strings.ReplaceAll(string(template), "[CODE]", op.Code)
	tempFile, err := ioutil.TempFile("", "*.py")
	if err != nil {
		panic(err)
	}
	defer os.Remove(tempFile.Name())
	if _, err := tempFile.Write([]byte(script)); err != nil {
		panic(err)
	}
	if err := tempFile.Close(); err != nil {
		panic(err)
	}
	cmd := exec.Command("/usr/bin/python", tempFile.Name())
	stdin, err := cmd.StdinPipe()
	if err != nil {
		panic(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		panic(err)
	}
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	defer stdout.Close()
	defer stdin.Close()
	defer stderr.Close()

	go printStderr("exec", stderr)

	go func() {
		for _, sliceInputs := range inputs {
			for _, input := range sliceInputs {
				bytes, err := json.Marshal(input)
				if err != nil {
					panic(err)
				}
				stdin.Write(bytes)
			}
		}
	}()

	rd := bufio.NewReader(stdout)
	outputs := make([]*LabeledClip, len(slices))
	for i, slice := range slices {
		var cur []interface{}
		for j := 0; j < len(inputs[i]); j++ {
			line, err := rd.ReadString('\n')
			if err != nil {
				log.Printf("[exec] error during python execution: %v", err)
				return nil
			}
			if op.Type == "track" {
				var track []Detection
				if err := json.Unmarshal([]byte(line), &track); err != nil {
					panic(err)
				}
				cur = append(cur, track)
			}
		}

		// merge the per-input results
		var label interface{}
		if op.Type == "track" {
			var tracks [][]Detection
			for _, x := range cur {
				track := x.([]Detection)
				tracks = append(tracks, track)
			}
			label = tracks
		}

		outputs = append(outputs, &LabeledClip{slice, op.Type, label})
	}

	return outputs
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

const NodeQuery = "SELECT n.id, n.query, v.id, v.name, v.ext, n.ls_id, n.type FROM nodes WHERE v.id = n.video_id"

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
func (node *Node) Test(slices []ClipSlice) []*LabeledClip {
	node.Load()
	// (1) if we have the labels in our LabelSet, then just return that
	// (2) TODO: if multiple Ops, then create node for each op and combine the outputs
	//     for now we only support single-op queries...
	// (3) otherwise call Test on our Op
	outputs := make([]*LabeledClip, len(slices))
	uniqueClips := make(map[int][]int)
	for i, slice := range slices {
		uniqueClips[slice.Clip.ID] = append(uniqueClips[slice.Clip.ID], i)
	}
	var missingSlices []ClipSlice
	var missingIndexes []int // indexes in arg slices
	for _, indexes := range uniqueClips {
		ok := func() bool {
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
		}
	}
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
	http.HandleFunc("/exec/test", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		videoID, _ := strconv.Atoi("video_id")
		query := r.Form.Get("query")

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
		clip := video.ListClips()[0]
		end := clip.Frames-1
		if end >= VisualizeMaxFrames {
			end = VisualizeMaxFrames-1
		}
		slice := ClipSlice{clip, 0, end}
		clipLabel := node.Test([]ClipSlice{slice})[0]
		pc := clipLabel.LoadPreview()
		uuid := cache.Add(pc)
		log.Printf("[exec] test: cached preview with %d frames, uuid=%s", len(pc.Images), uuid)
		jsonResponse(w, VisualizeResponse{
			PreviewURL: fmt.Sprintf("/cache/preview?id=%s&type=jpeg", uuid),
			URL: fmt.Sprintf("/cache/view?id=%s&type=mp4", uuid),
			Width: slice.Clip.Width,
			Height: slice.Clip.Height,
			UUID: uuid,
		})
	})
}
