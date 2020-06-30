package vaas

import (
	"fmt"
)

type VideoData []Image
func (d VideoData) IsEmpty() bool {
	return false
}
func (d VideoData) Length() int {
	return len(d)
}
func (d VideoData) EnsureLength(length int) Data {
	for len(d) < length {
		d = append(d, d[len(d)-1])
	}
	return d
}
func (d VideoData) Slice(i, j int) Data {
	return d[i:j]
}
func (d VideoData) Append(other Data) Data {
	other_ := other.(VideoData)
	return append(d, other_...)
}
func (d VideoData) Encode() []byte {
	panic(fmt.Errorf("encode not supported on VideoData"))
}
func (d VideoData) Type() DataType {
	return VideoType
}

func init() {
	dataImpls[VideoType] = DataImpl{
		New: func() Data {
			return VideoData{}
		},
		Decode: func(bytes []byte) Data {
			panic(fmt.Errorf("unsupported"))
		},
	}
}
