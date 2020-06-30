package vaas

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

type SimpleBufferIOMeta struct {
	Freq int
}

// I/O begins with header containing the metadata (SimpleBufferIOMeta).
// Then it is a sequence of packets, each packet has a 5-byte header.
// First byte is 'e' for error or 'd' for data.
// header[1:5] is the length of the packet.
// Rest of packet is the error or data.
func (buf *SimpleBuffer) FromReader(r io.Reader) {
	hlen := make([]byte, 4)
	_, err := io.ReadFull(r, hlen)
	if err != nil {
		buf.Error(fmt.Errorf("error SimpleBuffer from io.Reader: %v", err))
		return
	}
	l := int(binary.BigEndian.Uint32(hlen))
	metaBytes := make([]byte, l)
	_, err = io.ReadFull(r, metaBytes)
	if err != nil {
		buf.Error(fmt.Errorf("error reading SimpleBuffer from io.Reader: %v", err))
		return
	}
	var meta SimpleBufferIOMeta
	JsonUnmarshal(metaBytes, &meta)
	buf.SetMeta(meta.Freq)

	header := make([]byte, 5)
	for {
		_, err := io.ReadFull(r, header)
		if err == io.EOF {
			break
		} else if err != nil {
			buf.Error(fmt.Errorf("error reading SimpleBuffer from io.Reader: %v", err))
			return
		}
		b := make([]byte, int(binary.BigEndian.Uint32(header[1:5])))
		_, err = io.ReadFull(r, b)
		if err != nil {
			buf.Error(fmt.Errorf("error reading SimpleBuffer from io.Reader: %v", err))
			return
		}
		if header[0] == 'd' {
			buf.Write(DecodeData(buf.Type(), b))
		} else if header[0] == 'e' {
			buf.Error(fmt.Errorf(string(b)))
		}
	}
	buf.Close()
}

func (buf *SimpleBuffer) ToWriter(w io.Writer) error {
	buf.waitForMeta()
	meta := SimpleBufferIOMeta{
		Freq: buf.freq,
	}
	metaBytes := JsonMarshal(meta)
	hlen := make([]byte, 4)
	binary.BigEndian.PutUint32(hlen, uint32(len(metaBytes)))
	w.Write(hlen)
	w.Write(metaBytes)

	pos := 0
	header := make([]byte, 5)
	for {
		data, err := buf.read(pos, FPS)
		if err == io.EOF {
			break
		} else if err != nil {
			errBytes := []byte(err.Error())
			header[0] = 'e'
			binary.BigEndian.PutUint32(header[1:5], uint32(len(errBytes)))
			w.Write(header)
			w.Write(errBytes)
			return err
		}
		pos += data.Length()
		bytes := data.Encode()
		header[0] = 'd'
		binary.BigEndian.PutUint32(header[1:5], uint32(len(bytes)))
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
