package skyhook

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync"
)

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

func NewSimpleBuffer(t DataType) *SimpleBuffer {
	if t == VideoType {
		panic(fmt.Errorf("SimpleBuffer should not be used for video"))
	}
	buf := &SimpleBuffer{
		buf: NewData(t),
	}
	buf.cond = sync.NewCond(&buf.mu)
	return buf
}

func (buf *SimpleBuffer) Buffer() DataBuffer {
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

func (buf *SimpleBuffer) SetMeta(freq int) {
	buf.mu.Lock()
	buf.freq = freq
	buf.cond.Broadcast()
	buf.mu.Unlock()
}

func (buf *SimpleBuffer) waitForMeta() {
	buf.mu.Lock()
	for buf.freq == 0 && buf.err == nil && !buf.done {
		buf.cond.Wait()
	}
	buf.mu.Unlock()
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
	}
}

func (buf *SimpleBuffer) FromReader(r io.Reader) {
	header := make([]byte, 4)
	_, err := io.ReadFull(r, header)
	if err != nil {
		buf.Error(fmt.Errorf("error reading binary: %v", err))
		return
	}
	freq := int(binary.BigEndian.Uint32(header))
	buf.SetMeta(freq)
	for {
		_, err := io.ReadFull(r, header)
		if err == io.EOF {
			break
		} else if err != nil {
			buf.Error(fmt.Errorf("error reading binary: %v", err))
			return
		}
		b := make([]byte, int(binary.BigEndian.Uint32(header)))
		_, err = io.ReadFull(r, b)
		if err != nil {
			buf.Error(fmt.Errorf("error reading binary: %v", err))
			return
		}
		buf.Write(DecodeData(buf.Type(), b))
	}
	buf.Close()
}

func (buf *SimpleBuffer) ToWriter(w io.Writer) error {
	buf.waitForMeta()
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(buf.freq))
	w.Write(header)

	pos := 0
	for {
		data, err := buf.read(pos, FPS)
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		pos += data.Length()
		bytes := data.Encode()
		binary.BigEndian.PutUint32(header, uint32(len(bytes)))
		w.Write(header)
		w.Write(bytes)
	}
	return nil
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
	if rd.freq == 0 {
		rd.buf.waitForMeta()
		rd.freq = rd.buf.freq
	}
	return rd.freq
}

func (rd *SimpleReader) Close() {}
