package builtins

import (
	"../vaas"

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
	slice vaas.Slice
	parents []vaas.DataReader
	w vaas.DataWriter
}

type PythonExecutor struct {
	query *vaas.Query
	node vaas.Node
	tempFile *os.File

	// cmd is set to nil and err to "closed" after Close call
	cmd *vaas.Cmd
	stdin io.WriteCloser
	stdout io.ReadCloser

	// slices for which we are waiting for outputs
	pending map[int]*pendingSlice
	counter int

	// error running python template
	// this error is returned to any future Run calls
	err error

	// lock on stdin
	writeLock sync.Mutex

	// lock on internal structures (pending, err, counter, etc.)
	mu sync.Mutex
}

func (e *PythonExecutor) writeJSONPacket(x interface{}) {
	bytes := vaas.JsonMarshal(x)
	buf := make([]byte, 5)
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(bytes)))
	buf[4] = 'j'
	e.stdin.Write(buf)
	e.stdin.Write(bytes)
}

func (e *PythonExecutor) writeVideoPacket(images []vaas.Image) {
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
		Type vaas.DataType
		Parents int
	}
	meta.Type = e.node.DataType
	meta.Parents = len(e.node.Parents)
	e.writeLock.Lock()
	e.writeJSONPacket(meta)
	e.writeLock.Unlock()
}

func (e *PythonExecutor) Run(ctx vaas.ExecContext) vaas.DataBuffer {
	parents, err := GetParents(ctx, e.node)
	if err != nil {
		return vaas.GetErrorBuffer(e.node.DataType, fmt.Errorf("python error reading parents: %v", err))
	}

	var w vaas.DataWriter
	if e.node.DataType == vaas.VideoType {
		w = vaas.NewVideoWriter()
	} else {
		w = vaas.NewSimpleBuffer(e.node.DataType)
	}

	go func() {
		slice := ctx.Slice
		freq := vaas.MinFreq(parents)
		w.SetMeta(freq)

		// prepare pendingSlice
		e.mu.Lock()
		if e.err != nil {
			w.Error(e.err)
			e.mu.Unlock()
			return
		}
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

		f := func(index int, datas []vaas.Data) error {
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
				if data.Type() == vaas.VideoType {
					vdata := data.(vaas.VideoData)
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
		err := vaas.ReadMultiple(slice.Length(), freq, parents, f)
		if err != nil {
			panic(fmt.Errorf("ReadMultiple error at node %s: %v", e.node.Name, err))
		}
		// we don't close w here since ReadLoop will close it
	}()

	return w.Buffer()
}

func (e *PythonExecutor) ReadLoop() {
	t := e.node.DataType

	header := make([]byte, 16)
	for {
		_, err := io.ReadFull(e.stdout, header)
		if err != nil {
			break
		}
		sliceIdx := int(binary.BigEndian.Uint32(header[0:4]))
		start := int(binary.BigEndian.Uint32(header[4:8]))
		end := int(binary.BigEndian.Uint32(header[8:12]))
		size := int(binary.BigEndian.Uint32(header[12:16]))
		buf := make([]byte, size)
		_, err = io.ReadFull(e.stdout, buf)
		if err != nil {
			break
		}
		var data vaas.Data
		if t == vaas.VideoType {
			nframes := int(binary.BigEndian.Uint32(buf[0:4]))
			height := int(binary.BigEndian.Uint32(buf[4:8]))
			width := int(binary.BigEndian.Uint32(buf[8:12]))
			// TODO: channels buf[12:16]
			chunkSize := width*height*3
			buf = buf[16:]
			var vdata vaas.VideoData
			for i := 0; i < nframes; i++ {
				vdata = append(vdata, vaas.ImageFromBytes(width, height, buf[i*chunkSize:(i+1)*chunkSize]))
			}
			data = vdata
		} else {
			data = vaas.DecodeData(t, buf)
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

	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.pending) == 0 && e.err != nil && e.err.Error() == "closed" {
		return
	}
	if e.cmd != nil {
		e.err = e.cmd.Wait()
		e.cmd = nil
	}
	if e.err == nil {
		e.err = fmt.Errorf("cmd closed unexpectdly")
	}
	log.Printf("[python (%s)] error during python execution: %v", e.node.Name, e.err)
	for _, ps := range e.pending {
		ps.w.Error(e.err)
	}
}

func (e *PythonExecutor) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stdin.Close()
	e.stdout.Close()
	if e.cmd != nil {
		e.cmd.Wait()
		e.cmd = nil
		e.err = fmt.Errorf("closed")
	}
	os.Remove(e.tempFile.Name())
}

func NewPythonExecutor(node vaas.Node) vaas.Executor {
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
	cmd := vaas.Command(
		fmt.Sprintf("exec-python-%s", node.Name), vaas.CommandOptions{},
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
	vaas.Executors["python"] = vaas.ExecutorMeta{New: NewPythonExecutor}
}
