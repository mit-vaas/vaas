package skyhook

import (
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
	slice Slice
	parents []DataReader
	buf DataBuffer
}

type PythonExecutor struct {
	query *Query
	node *Node
	tempFile *os.File
	cmd Cmd
	stdin io.WriteCloser
	stdout io.ReadCloser
	pending map[int]*pendingSlice
	counter int

	writeLock sync.Mutex
	mu sync.Mutex
}

func (e *PythonExecutor) writeJSONPacket(x interface{}) {
	bytes := JsonMarshal(x)
	buf := make([]byte, 5)
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(bytes)))
	buf[4] = 'j'
	e.stdin.Write(buf)
	e.stdin.Write(bytes)
}

func (e *PythonExecutor) writeVideoPacket(images []Image) {
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
		Type DataType
		Parents int
	}
	meta.Type = e.node.Type
	meta.Parents = len(e.node.Parents)
	e.writeLock.Lock()
	e.writeJSONPacket(meta)
	e.writeLock.Unlock()
}

func (e *PythonExecutor) Run(parents []DataReader, slice Slice) DataBuffer {
	// prepare pendingSlice
	e.mu.Lock()
	id := e.counter
	e.counter++
	var buf DataBuffer
	if e.node.Type == VideoType {
		buf = NewVideoBuffer()
	} else {
		buf = NewSimpleBuffer(e.node.Type)
	}
	ps := &pendingSlice{slice, parents, buf}
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

	// write parent data asynchronously
	go func() {
		f := func(index int, datas []Data) error {
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
				if data.Type() == VideoType {
					vdata := data.(VideoData)
					e.writeVideoPacket(vdata)
				} else {
					e.writeJSONPacket(data)
				}
			}
			e.writeLock.Unlock()
			return nil
		}
		ReadMultiple(slice.Length(), parents, f)
	}()

	return buf
}

func (e *PythonExecutor) ReadLoop() {
	t := e.node.Type

	setErr := func(err error) {
		e.mu.Lock()
		defer e.mu.Unlock()
		if len(e.pending) == 0 {
			return
		}
		log.Printf("[exec (%s/%s)] error during python execution: %v", e.query.Name, e.node.Name, err)
		for _, ps := range e.pending {
			ps.buf.Error(err)
		}
	}

	header := make([]byte, 16)
	for {
		_, err := io.ReadFull(e.stdout, header)
		if err != nil {
			setErr(err)
			return
		}
		sliceIdx := int(binary.BigEndian.Uint32(header[0:4]))
		start := int(binary.BigEndian.Uint32(header[4:8]))
		end := int(binary.BigEndian.Uint32(header[8:12]))
		size := int(binary.BigEndian.Uint32(header[12:16]))
		buf := make([]byte, size)
		_, err = io.ReadFull(e.stdout, buf)
		if err != nil {
			setErr(err)
			return
		}
		var data Data
		if t == VideoType {
			nframes := int(binary.BigEndian.Uint32(buf[0:4]))
			height := int(binary.BigEndian.Uint32(buf[4:8]))
			width := int(binary.BigEndian.Uint32(buf[8:12]))
			// TODO: channels buf[12:16]
			chunkSize := width*height*3
			buf = buf[16:]
			var vdata VideoData
			for i := 0; i < nframes; i++ {
				vdata = append(vdata, ImageFromBytes(width, height, buf[i*chunkSize:(i+1)*chunkSize]))
			}
			data = vdata
		} else {
			data = DecodeData(t, buf)
		}
		data = data.EnsureLength(end-start)

		e.mu.Lock()
		ps := e.pending[sliceIdx]
		ps.buf.Write(data)
		if end >= ps.slice.Length() {
			ps.buf.Close()
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

func (node *Node) pythonExecutor(query *Query) Executor {
	log.Printf("[exec (%s/%s)] launching python script", query.Name, node.Name)
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
	cmd := Command(
		fmt.Sprintf("exec-python-%s", node.Name), CommandOptions{},
		"/usr/bin/python3", tempFile.Name(),
	)

	e := &PythonExecutor{
		query: query,
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
