package skyhook

import (
	"fmt"
	"io"
	"sync"
)

type DataType string
const (
	DetectionType DataType = "detection"
	TrackType = "track"
	ClassType = "class"
	VideoType = "video"
	TextType = "text"
)

type DataImpl struct {
	New func() Data
	Decode func([]byte) Data
}

var dataImpls = make(map[DataType]DataImpl)

type Data interface {
	IsEmpty() bool
	Length() int
	EnsureLength(length int) Data
	Slice(i, j int) Data
	Append(other Data) Data
	Encode() []byte
	Type() DataType
}

func NewData(t DataType) Data {
	return dataImpls[t].New()
}

func DecodeData(t DataType, bytes []byte) Data {
	return dataImpls[t].Decode(bytes)
}

type DataReader interface {
	Type() DataType

	// Reads up to n frames.
	// Must only return io.EOF if no Data is returned.
	Read(n int) (Data, error)

	// Like Read but the returned Data will be returned again on the next Read/Peek.
	Peek(n int) (Data, error)

	// sampling frequency of this buffer (1 means no downsampling).
	// For a slice of length N frames, the length if the data from this DataReader
	// should be (N+Freq-1)/Freq (not N/freq because e.g. N=15 Freq=16 should still
	// produce one partial output).
	Freq() int

	Close()
}

type DataBuffer interface {
	Type() DataType
	Reader() DataReader
	Wait() error
}

type DataWriter interface {
	DataBuffer
	Write(data Data)
	Close()
	Error(err error)
}

type SimpleBuffer struct {
	buf Data
	mu sync.Mutex
	cond *sync.Cond
	err error
	done bool
	freq int
	length int
}

type SimpleReader struct {
	buf *SimpleBuffer
	freq int
	pos int
}

func NewSimpleBuffer(t DataType, freq int) *SimpleBuffer {
	if t == VideoType {
		panic(fmt.Errorf("SimpleBuffer should not be used for video"))
	} else if freq <= 0 {
		panic(fmt.Errorf("freq must be positive"))
	}
	buf := &SimpleBuffer{
		buf: NewData(t),
		freq: freq,
	}
	buf.cond = sync.NewCond(&buf.mu)
	return buf
}

// Optionally set the length of this buffer.
// The buffer will drop Writes or duplicate items on Close to ensure it matches the length.
func (buf *SimpleBuffer) SetLength(length int) {
	buf.length = length
}

func (buf *SimpleBuffer) Type() DataType {
	return buf.buf.Type()
}

func (buf *SimpleBuffer) read(pos int, n int) (Data, error) {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	wait := n
	if wait <= 0 {
		wait = 1
	}
	for buf.buf.Length() < pos+wait && buf.err == nil && !buf.done {
		buf.cond.Wait()
	}
	if buf.err != nil {
		return nil, buf.err
	} else if pos == buf.buf.Length() && buf.done {
		return nil, io.EOF
	}

	if n <= 0 || pos+n > buf.buf.Length() {
		n = buf.buf.Length()-pos
	}
	data := NewData(buf.Type()).Append(buf.buf.Slice(pos, pos+n))
	return data, nil
}

// Wait for at least n to complete, and read them without removing from the buffer.
func (buf *SimpleBuffer) peek(pos int, n int) (Data, error) {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	if n > 0 {
		for buf.buf.Length() < pos+n && buf.err == nil && !buf.done {
			buf.cond.Wait()
		}
	}
	if buf.err != nil {
		return nil, buf.err
	}
	data := NewData(buf.Type()).Append(buf.buf.Slice(pos, buf.buf.Length()))
	return data, nil
}

func (buf *SimpleBuffer) Write(data Data) {
	buf.mu.Lock()
	buf.buf = buf.buf.Append(data)
	if buf.length > 0 && buf.buf.Length() > buf.length {
		buf.buf = buf.buf.Slice(0, buf.length)
	}
	buf.cond.Broadcast()
	buf.mu.Unlock()
}

func (buf *SimpleBuffer) Close() {
	buf.mu.Lock()
	if buf.length > 0 && buf.buf.Length() < buf.length {
		buf.buf = buf.buf.EnsureLength(buf.length)
	}
	buf.done = true
	buf.cond.Broadcast()
	buf.mu.Unlock()
}

func (buf *SimpleBuffer) Error(err error) {
	buf.mu.Lock()
	buf.err = err
	buf.cond.Broadcast()
	buf.mu.Unlock()
}

func (buf *SimpleBuffer) Wait() error {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	for !buf.done && buf.err == nil {
		buf.cond.Wait()
	}
	if buf.err != nil {
		return buf.err
	}
	return nil
}

func (buf *SimpleBuffer) Reader() DataReader {
	return &SimpleReader{
		buf: buf,
		freq: buf.freq,
	}
}

func (rd *SimpleReader) Type() DataType {
	return rd.buf.Type()
}

func (rd *SimpleReader) Read(n int) (Data, error) {
	data, err := rd.buf.read(rd.pos, n)
	if data != nil {
		rd.pos += data.Length()
	}
	return data, err
}

func (rd *SimpleReader) Peek(n int) (Data, error) {
	return rd.buf.peek(rd.pos, n)
}

func (rd *SimpleReader) Freq() int {
	return rd.freq
}

func (rd *SimpleReader) Close() {}

func MinFreq(inputs []DataReader) int {
	freq := inputs[0].Freq()
	for _, input := range inputs {
		if input.Freq() < freq {
			freq = input.Freq()
		}
	}
	return freq
}

// Duplicate or remove items to adjust the data's freq to target.
// Length is the length at freq 1.
func AdjustDataFreq(data Data, length int, freq int, target int) Data {
	if freq == target {
		return data
	}
	out := NewData(data.Type())
	olen := (length + target-1) / target
	for i := 0; i < olen; i++ {
		idx := i*target/freq
		out = out.Append(data.Slice(idx, idx+1))
	}
	return out
}

// Read from multiple DataReaders in lockstep.
// Output is limited by the slowest DataReader.
// Also adjusts input data so that the Data provided to callback corresponds to targetFreq.
func ReadMultiple(length int, targetFreq int, inputs []DataReader, callback func(int, []Data) error) error {
	defer func() {
		for _, input := range inputs {
			input.Close()
		}
	}()

	// compute the number of frames to read each iteration
	// this depends on the sampling rate since reads should be aligned with the highest freq
	// note that on the last iteration, we may do a partial read on the highest freq readers
	perIter := FPS
	for _, input := range inputs {
		perIter = (perIter / input.Freq()) * input.Freq()
	}
	if perIter == 0 {
		panic(fmt.Errorf("could not find suitable per-iteration read size"))
	}

	completed := 0
	for completed < length {
		// peek each input to see how much we can read
		available := -1
		for i, rd := range inputs {
			data, err := rd.Peek(perIter/rd.Freq())
			if err != nil {
				return fmt.Errorf("error from input peek: %v (from input idx %d type %v)", err, i, rd.Type())
			}
			if available == -1 || data.Length()*rd.Freq() < available {
				available = data.Length()*rd.Freq()
			}
		}

		// round available up to nearest multiple of perIter, but can't go past length
		available = ((available + perIter-1) / perIter) * perIter
		if completed+available > length {
			available = length - completed
		}

		datas := make([]Data, len(inputs))
		for i, rd := range inputs {
			// if available is not a multiple of rd.Freq, it implies we're on the last
			// output and we need to make sure we capture the partial output frame
			data, err := rd.Read((available+rd.Freq()-1)/rd.Freq())
			if err != nil {
				return fmt.Errorf("error from input read: %v (from input idx %d type %v)", err, i, rd.Type())
			}
			datas[i] = AdjustDataFreq(data, available, rd.Freq(), targetFreq)
		}

		err := callback(completed, datas)
		if err != nil {
			return fmt.Errorf("error from callback: %v", err)
		}

		completed += available
	}

	return nil
}
