package vaas

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
)

// we assume all videos are constant 25 fps
// TODO: use per-clip or per-video fps
// re-encoding is likely needed to work with skyhook (if video has variable framerate)
// ffmpeg seeking is only fast and frame-accurate with constant framerate
const FPS int = 25

type Timeline struct {
	ID int
	Name string
}

type Series struct {
	ID int
	Timeline Timeline
	Name string
	Type string
	DataType DataType

	// for labels/outputs
	SrcVector []Series
	// for outputs
	Node *Node
	// for data, during ingestion
	Percent int
}

type Segment struct {
	ID int
	Timeline Timeline
	Name string
	Frames int
	FPS float64
}

func (segment Segment) ToSlice() Slice {
	return Slice{segment, 0, segment.Frames}
}

type Slice struct {
	Segment Segment
	Start int
	End int
}

func (slice Slice) String() string {
	return fmt.Sprintf("%d[%d:%d]", slice.Segment.ID, slice.Start, slice.End)
}

func (slice Slice) Length() int {
	return slice.End - slice.Start
}

func (slice Slice) Equals(other Slice) bool {
	return slice.Segment.ID == other.Segment.ID && slice.Start == other.Start && slice.End == other.End
}

type Item struct {
	ID int
	Slice Slice
	Series Series
	Format string

	Width int
	Height int
	Freq int
}

func pad6(x int) string {
	s := fmt.Sprintf("%d", x)
	for len(s) < 6 {
		s = "0" + s
	}
	return s
}

func (item Item) Fname(index int) string {
	if item.Format == "jpeg" {
		return fmt.Sprintf("items/%d/%d/%s.jpg", item.Series.ID, item.ID, pad6(index))
	} else {
		// json / mp4 (and other video formats)
		return fmt.Sprintf("items/%d/%d.%s", item.Series.ID, item.ID, item.Format)
	}
}

func (item Item) Load(slice Slice) DataBuffer {
	if slice.Segment.ID != item.Slice.Segment.ID || slice.Start < item.Slice.Start || slice.End > item.Slice.End {
		return nil
	}
	if item.Series.DataType == VideoType {
		return VideoFileBuffer{item, slice}
	}
	buf := NewSimpleBuffer(item.Series.DataType)
	buf.SetMeta(item.Freq)
	go func() {
		bytes, err := ioutil.ReadFile(item.Fname(0))
		if err != nil {
			log.Printf("[item] error loading %d/%d: %v", item.Series.ID, item.ID, err)
			buf.Error(err)
			return
		}
		data := DecodeData(item.Series.DataType, bytes)
		data = data.Slice((slice.Start - item.Slice.Start)/item.Freq, (slice.End - item.Slice.Start + item.Freq-1)/item.Freq)
		buf.Write(data)
		buf.Close()
	}()
	return buf
}

func (item Item) Mkdir() {
	os.Mkdir(fmt.Sprintf("items/%d", item.Series.ID), 0755)
	if item.Format == "jpeg" {
		os.Mkdir(fmt.Sprintf("items/%d/%d", item.Series.ID, item.ID), 0755)
	}
}

func (item Item) UpdateData(data Data) {
	item.Mkdir()
	if err := ioutil.WriteFile(item.Fname(0), data.Encode(), 0644); err != nil {
		panic(err)
	}
}
