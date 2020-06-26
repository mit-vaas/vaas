package builtins

import (
	"../skyhook"

	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"
)

type pendingSlice struct {
	slice skyhook.Slice
	parents []skyhook.DataReader
	w skyhook.DataWriter
}

type PythonExecutor struct {
	query *skyhook.Query
	node *skyhook.Node
	tempFile *os.File
	cmd *skyhook.Cmd
	stdin io.WriteCloser
	stdout io.ReadCloser
	pending map[int]*pendingSlice
	counter int

	writeLock sync.Mutex
	mu sync.Mutex
}

func (e *PythonExecutor) writeJSONPacket(x interface{}) {
	bytes := skyhook.JsonMarshal(x)
	buf := make([]byte, 5)
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(bytes)))
	buf[4] = 'j'
	e.stdin.Write(buf)
	e.stdin.Write(bytes)
}

func (e *PythonExecutor) writeVideoPacket(images []skyhook.Image) {
	buf := make([]byte, 21)
	l := 16+len(images)*images[0].Width*images[0].Height*3
	binary.BigEndian.PutUint32(buf[0:4], uint32(l))
	buf[4] = 'v'
	binary.BigEndian.PutUint32(buf[5:9], uint32(len(images)))
	binary.BigEndian.PutUint32(buf[9:13], uint32(images[0].Height))
	binary.BigEndian.PutUint32(buf[13:17], uint32(images[0].Width))
	binary.BigEndian.PutUint32(buf[17:21], 3)
	e.stdin.Write(buf)
	for _, image := range images {
		e.stdin.Write(image.ToBytes())
	}
}

func (e *PythonExecutor) Init() {
	// prepare meta
	var meta struct {
		Type skyhook.DataType
		Parents int
	}
	meta.Type = e.node.DataType
	meta.Parents = len(e.node.Parents)
	e.writeLock.Lock()
	e.writeJSONPacket(meta)
	e.writeLock.Unlock()
}

func (e *PythonExecutor) Run(ctx skyhook.ExecContext) skyhook.DataBuffer {
	parents, err := GetParents(ctx, e.node)
	if err != nil {
		return skyhook.GetErrorBuffer(e.node.DataType, fmt.Errorf("python error reading parents: %v", err))
	}

	var w skyhook.DataWriter
	if e.node.DataType == skyhook.VideoType {
		w = skyhook.NewVideoWriter()
	} else {
		w = skyhook.NewSimpleBuffer(e.node.DataType)
	}

	go func() {
		slice := ctx.Slice
		freq := skyhook.MinFreq(parents)
		w.SetMeta(freq)

		// prepare pendingSlice
		e.mu.Lock()
		id := e.counter
		e.counter++
		ps := &pendingSlice{slice, parents, w}
		e.pending[id] = ps
		e.mu.Unlock()

		// write init packet
		var initPacket struct {
			ID int
			Length int
		}
		initPacket.ID = id
		initPacket.Length = slice.Length()
		e.writeLock.Lock()
		e.writeJSONPacket(initPacket)
		e.writeLock.Unlock()

		f := func(index int, datas []skyhook.Data) error {
			var job struct {
				SliceIdx int
				Range [2]int
				IsLast bool
			}
			job.SliceIdx = id
			job.Range = [2]int{index, index+datas[0].Length()}
			job.IsLast = job.Range[1] == slice.Length()

			e.writeLock.Lock()
			e.writeJSONPacket(job)
			for _, data := range datas {
				if data.Type() == skyhook.VideoType {
					vdata := data.(skyhook.VideoData)
					e.writeVideoPacket(vdata)
				} else {
					e.writeJSONPacket(data)
				}
			}
			e.writeLock.Unlock()
			return nil
		}
		// TODO: look at the return error
		// currently we don't because w is controlled by ReadLoop
		err := skyhook.ReadMultiple(slice.Length(), freq, parents, f)
		if err != nil {
			panic(fmt.Errorf("ReadMultiple error at node %s: %v", e.node.Name, err))
		}
		// we don't close w here since ReadLoop will close it
	}()

	return w.Buffer()
}

func (e *PythonExecutor) ReadLoop() {
	t := e.node.DataType

	setErr := func(err error) {
		e.mu.Lock()
		defer e.mu.Unlock()
		if len(e.pending) == 0 {
			return
		}
		log.Printf("[python (%s)] error during python execution: %v", e.node.Name, err)
		for _, ps := range e.pending {
			ps.w.Error(err)
		}
	}

	header := make([]byte, 16)
	for {
		_, err := io.ReadFull(e.stdout, header)
		if err != nil {
			setErr(fmt.Errorf("error reading from python: %v", err))
			return
		}
		sliceIdx := int(binary.BigEndian.Uint32(header[0:4]))
		start := int(binary.BigEndian.Uint32(header[4:8]))
		end := int(binary.BigEndian.Uint32(header[8:12]))
		size := int(binary.BigEndian.Uint32(header[12:16]))
		buf := make([]byte, size)
		_, err = io.ReadFull(e.stdout, buf)
		if err != nil {
			setErr(fmt.Errorf("error reading from python: %v", err))
			return
		}
		var data skyhook.Data
		if t == skyhook.VideoType {
			nframes := int(binary.BigEndian.Uint32(buf[0:4]))
			height := int(binary.BigEndian.Uint32(buf[4:8]))
			width := int(binary.BigEndian.Uint32(buf[8:12]))
			// TODO: channels buf[12:16]
			chunkSize := width*height*3
			buf = buf[16:]
			var vdata skyhook.VideoData
			for i := 0; i < nframes; i++ {
				vdata = append(vdata, skyhook.ImageFromBytes(width, height, buf[i*chunkSize:(i+1)*chunkSize]))
			}
			data = vdata
		} else {
			data = skyhook.DecodeData(t, buf)
		}
		data = data.EnsureLength(end-start)

		e.mu.Lock()
		ps := e.pending[sliceIdx]
		ps.w.Write(data)
		if end >= ps.slice.Length() {
			ps.w.Close()
			delete(e.pending, sliceIdx)
		}
		e.mu.Unlock()
	}
}

func (e *PythonExecutor) Close() {
	e.stdin.Close()
	e.stdout.Close()
	e.cmd.Wait()
	os.Remove(e.tempFile.Name())
}

func NewPythonExecutor(node *skyhook.Node) skyhook.Executor {
	log.Printf("[python (%s)] launching python script", node.Name)
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
	cmd := skyhook.Command(
		fmt.Sprintf("exec-python-%s", node.Name), skyhook.CommandOptions{},
		"/usr/bin/python3", tempFile.Name(),
	)

	e := &PythonExecutor{
		node: node,
		tempFile: tempFile,
		cmd: cmd,
		stdin: cmd.Stdin(),
		stdout: cmd.Stdout(),
		pending: make(map[int]*pendingSlice),
	}
	e.Init()
	go e.ReadLoop()
	return e
}

func init() {
	skyhook.Executors["python"] = NewPythonExecutor
}
