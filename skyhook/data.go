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
)

type Detection struct {
	Left int `json:"left"`
	Top int `json:"top"`
	Right int `json:"right"`
	Bottom int `json:"bottom"`
	Score float64 `json:"score,omitempty"`
	Class string `json:"class,omitempty"`
	TrackID int `json:"track_id"`
}

func DetectionsToTracks(detections [][]Detection) [][]Detection {
	tracks := make(map[int][]Detection)
	for frameIdx := range detections {
		for _, detection := range detections[frameIdx] {
			if detection.TrackID < 0 {
				continue
			}
			tracks[detection.TrackID] = append(tracks[detection.TrackID], detection)
		}
	}
	var trackList [][]Detection
	for _, track := range tracks {
		trackList = append(trackList, track)
	}
	return trackList
}

type Data struct {
	Type DataType
	Detections [][]Detection
	Classes []int
	Images []Image
}

func (d Data) Nil() bool {
	return d.Type == ""
}

func (d Data) Length() int {
	if d.Type == DetectionType || d.Type == TrackType {
		return len(d.Detections)
	} else if d.Type == ClassType {
		return len(d.Classes)
	} else if d.Type == VideoType {
		return len(d.Images)
	}
	panic(fmt.Errorf("bad type %v", d.Type))
}

func (d Data) EnsureLength(length int) Data {
	if d.Type == DetectionType || d.Type == TrackType {
		for i := len(d.Detections); i < length; i++ {
			d.Detections = append(d.Detections, nil)
		}
	} else if d.Type == ClassType {
		for i := len(d.Classes); i < length; i++ {
			d.Classes = append(d.Classes, 0)
		}
	}
	return d
}

func (d Data) Slice(i, j int) Data {
	if d.Type == DetectionType || d.Type == TrackType {
		return Data{
			Type: d.Type,
			Detections: d.Detections[i:j],
		}
	} else if d.Type == ClassType {
		return Data{
			Type: d.Type,
			Classes: d.Classes[i:j],
		}
	} else if d.Type == VideoType {
		return Data{
			Type: d.Type,
			Images: d.Images[i:j],
		}
	}
	panic(fmt.Errorf("bad type %v", d.Type))
}

func (d Data) Append(other Data) Data {
	if d.Type == DetectionType || d.Type == TrackType {
		return Data{
			Type: d.Type,
			Detections: append(d.Detections, other.Detections...),
		}
	} else if d.Type == ClassType {
		return Data{
			Type: d.Type,
			Classes: append(d.Classes, other.Classes...),
		}
	} else if d.Type == VideoType {
		return Data{
			Type: d.Type,
			Images: append(d.Images, other.Images...),
		}
	}
	panic(fmt.Errorf("bad type %v", d.Type))
}

func (d Data) Get() interface{} {
	if d.Type == DetectionType || d.Type == TrackType {
		return d.Detections
	} else if d.Type == ClassType {
		return d.Classes
	} else if d.Type == VideoType {
		return d.Images
	}
	panic(fmt.Errorf("bad type %v", d.Type))
}

func (d Data) IsEmpty() bool {
	if d.Type == DetectionType || d.Type == TrackType {
		for _, dlist := range d.Detections {
			if len(dlist) > 0 {
				return false
			}
		}
		return true
	} else if d.Type == ClassType {
		for _, class := range d.Classes {
			if class > 0 {
				return false
			}
		}
		return true
	} else if d.Type == VideoType {
		return false
	}
	panic(fmt.Errorf("bad type %v", d.Type))
}

func (d Data) Discard(n int) Data {
	if d.Type == DetectionType || d.Type == TrackType {
		copy(d.Detections[0:], d.Detections[n:])
		d.Detections = d.Detections[0:len(d.Detections)-n]
		return d
	} else if d.Type == ClassType {
		copy(d.Classes[0:], d.Classes[n:])
		d.Classes = d.Classes[0:len(d.Classes)-n]
		return d
	} else if d.Type == VideoType {
		copy(d.Images[0:], d.Images[n:])
		d.Images = d.Images[0:len(d.Images)-n]
		return d
	}
	panic(fmt.Errorf("bad type %v", d.Type))
}

type LabelBuffer struct {
	buf Data

	// # frames that were discarded
	// (data is discarded once all readers read it)
	pos int

	// reader positions
	rdpos []int

	mu sync.Mutex
	cond *sync.Cond
	err error
	done bool

	// soft maximum on the # frames in the buffer
	capacity int

	// are we paused (waiting to register readers)
	paused bool
}

type BufferReader struct {
	buf *LabelBuffer
	id int
}

func NewLabelBuffer(t DataType) *LabelBuffer {
	buf := &LabelBuffer{buf: Data{Type: t}}
	buf.cond = sync.NewCond(&buf.mu)
	if t == VideoType {
		// ideally this would be set somewhere else
		// but we set a cap so the frames don't eat up all the memory
		buf.capacity = FPS
	}
	return buf
}

// caller must have lock
func (buf *LabelBuffer) length() int {
	return buf.pos + buf.buf.Length()
}

// caller must have lock
func (buf *LabelBuffer) discard() {
	npos := buf.rdpos[0]
	for _, x := range buf.rdpos {
		if x < 0 {
			// this reader closed
			continue
		}
		if x < npos {
			npos = x
		}
	}
	if npos <= buf.pos {
		return
	}
	buf.buf = buf.buf.Discard(npos - buf.pos)
	buf.pos = npos
	buf.cond.Broadcast()
}

// Read exactly n frames, or any non-zero amount if n<=0
func (buf *LabelBuffer) read(id int, n int) (Data, error) {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	wait := n
	if wait <= 0 {
		wait = 1
	}
	for (buf.length() < buf.rdpos[id]+wait || buf.paused) && buf.err == nil && !buf.done {
		buf.cond.Wait()
	}
	if buf.err != nil {
		return Data{}, buf.err
	} else if buf.buf.Length() == 0 && buf.done {
		return Data{}, io.EOF
	} else if buf.buf.Length() < n {
		return Data{}, io.ErrUnexpectedEOF
	}

	if n <= 0 {
		n = buf.length()-buf.rdpos[id]
	}
	offset := buf.rdpos[id] - buf.pos
	data := Data{
		Type: buf.Type(),
	}.Append(buf.buf.Slice(offset, offset+n))
	buf.rdpos[id] += n
	buf.discard()
	return data, nil
}

// Wait for at least n to complete, and read them without removing from the buffer.
func (buf *LabelBuffer) peek(id int, n int) (Data, error) {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	if n > 0 {
		for buf.length() < buf.rdpos[id]+n && buf.err == nil && !buf.done {
			buf.cond.Wait()
		}
	}
	if buf.err != nil {
		return Data{}, buf.err
	}
	offset := buf.rdpos[id] - buf.pos
	data := Data{
		Type: buf.Type(),
	}.Append(buf.buf.Slice(offset, buf.buf.Length()))
	return data, nil
}

func (buf *LabelBuffer) Write(data Data) {
	buf.mu.Lock()
	defer buf.mu.Unlock()
	for buf.capacity > 0 && buf.buf.Length() > 0 && buf.buf.Length()+data.Length() > buf.capacity && buf.err == nil && !buf.done {
		buf.cond.Wait()
	}
	if buf.err != nil || buf.done {
		return
	}
	buf.buf = buf.buf.Append(data)
	buf.cond.Broadcast()
}

func (buf *LabelBuffer) Close() {
	buf.mu.Lock()
	buf.done = true
	buf.cond.Broadcast()
	buf.mu.Unlock()
}

func (buf *LabelBuffer) Error(err error) {
	buf.mu.Lock()
	buf.err = err
	buf.cond.Broadcast()
	buf.mu.Unlock()
}

func (buf *LabelBuffer) FromVideoReader(rd VideoReader) {
	for {
		im, err := rd.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			panic(err)
		}
		buf.Write(Data{
			Type: VideoType,
			Images: []Image{im},
		})
	}
	buf.Close()
}

func (buf *LabelBuffer) Type() DataType {
	return buf.buf.Type
}

func (buf *LabelBuffer) CopyFrom(length int, rd *BufferReader) error {
	cur := 0
	for cur < length {
		data, err := rd.Read(0)
		if err != nil {
			buf.Error(err)
			return err
		}
		cur += data.Length()
		buf.Write(data)
	}
	buf.Close()
	return nil
}

func (buf *LabelBuffer) SetPaused(paused bool) {
	buf.mu.Lock()
	buf.paused = paused
	buf.cond.Broadcast()
	buf.mu.Unlock()
}

func (buf *LabelBuffer) Reader() *BufferReader {
	buf.mu.Lock()
	if buf.pos > 0 {
		panic(fmt.Errorf("tried to add reader when reading already started"))
	}
	id := len(buf.rdpos)
	buf.rdpos = append(buf.rdpos, 0)
	buf.mu.Unlock()
	return &BufferReader{buf, id}
}

func (rd *BufferReader) Type() DataType {
	return rd.buf.Type()
}

func (rd *BufferReader) Read(n int) (Data, error) {
	return rd.buf.read(rd.id, n)
}

func (rd *BufferReader) Peek(n int) (Data, error) {
	return rd.buf.peek(rd.id, n)
}

func (rd *BufferReader) Wait(length int) error {
	completed := 0
	for completed < length {
		data, err := rd.Read(0)
		if err != nil {
			return err
		}
		completed += data.Length()
	}
	return nil
}

func (rd *BufferReader) Close() {
	// tell buffer that we read everything so that it doesn't wait on us
	rd.buf.mu.Lock()
	rd.buf.rdpos[rd.id] = -1
	rd.buf.discard()
	rd.buf.mu.Unlock()
}

// TODO: there could be issues due to buffer capacity if inputs have mutual dependencies
// e.g. inputs[0] is video and inputs[1] depends on inputs[0]
// additionally, suppose inputs[1] is NOT streaming (waits for entire inputs[0] before outputting data)
// then below we would get stuck since inputs[0] is capacity limited and inputs[1] isn't producing anything
// probably solution is to cache the video (either as images or video) if a descendant operation is not streaming
func ReadMultiple(length int, inputs []*BufferReader, callback func(int, []Data) error) error {
	defer func() {
		for _, input := range inputs {
			input.Close()
		}
	}()

	completed := 0
	for completed < length {
		// peek each input to see how much we can read
		available := -1
		for _, rd := range inputs {
			data, err := rd.Peek(1)
			if err != nil {
				return err
			}
			if available == -1 || data.Length() < available {
				available = data.Length()
			}
		}

		datas := make([]Data, len(inputs))
		for i, rd := range inputs {
			data, err := rd.Read(available)
			if err != nil {
				return err
			}
			datas[i] = data
		}

		err := callback(completed, datas)
		if err != nil {
			return err
		}

		completed += available
	}

	return nil
}

// Returns a new buffer that reads from this buffer without removing the read
// content. This allows single-writer multi-reader mode, where all the readers
// read from different splits of the original buffer.
// This also optionally slices the buffer so that it only yields data in [start, end).
/*func (buf *LabelBuffer) Slice(start int, end int) *LabelBuffer {
	nbuf := NewLabelBuffer(buf.unread.Type)
	go func() {
		cur := start
		iter := func() bool {
			buf.mu.Lock()
			defer buf.mu.Unlock()
			for buf.unread.Length() <= cur && buf.err == nil && !buf.done {
				buf.cond.Wait()
			}
			if buf.err != nil {
				nbuf.Error(fmt.Errorf("passing error to downstream split buffer: %v", buf.err))
				return true
			} else if buf.unread.Length() <= cur && buf.done {
				nbuf.mu.Lock()
				nbuf.done = true
				nbuf.mu.Unlock()
				return true
			}

		}
		for cur < end {
			quit := iter()
			if quit {
				break
			}
		}
	}()
	return nbuf
}*/
