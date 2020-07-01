package vaas

import (
	"bytes"
	"encoding/binary"
)

// Associates a list of images with each frame.
// These images are encoded as JPGs in a binary multi-image format.

type ImListData [][]Image
func (d ImListData) IsEmpty() bool {
	for _, imlist := range d {
		if len(imlist) > 0 {
			return false
		}
	}
	return true
}
func (d ImListData) Length() int {
	return len(d)
}
func (d ImListData) EnsureLength(length int) Data {
	for len(d) < length {
		d = append(d, []Image{})
	}
	return d
}
func (d ImListData) Slice(i, j int) Data {
	return d[i:j]
}
func (d ImListData) Append(other Data) Data {
	other_ := other.(ImListData)
	return append(d, other_...)
}
func (d ImListData) Encode() []byte {
	header := make([]byte, 4)
	buf := new(bytes.Buffer)
	binary.BigEndian.PutUint32(header, uint32(len(d)))
	buf.Write(header)
	for _, imlist := range d {
		binary.BigEndian.PutUint32(header, uint32(len(imlist)))
		buf.Write(header)
		for _, im := range imlist {
			imbytes := im.AsJPG()
			binary.BigEndian.PutUint32(header, uint32(len(imbytes)))
			buf.Write(header)
			buf.Write(imbytes)
		}
	}
	return buf.Bytes()
}
func (d ImListData) Type() DataType {
	return ImListType
}

func init() {
	dataImpls[ImListType] = DataImpl{
		New: func() Data {
			return ImListData{}
		},
		Decode: func(bin []byte) Data {
			header := make([]byte, 4)
			buf := bytes.NewBuffer(bin)
			buf.Read(header)
			d := make(ImListData, int(binary.BigEndian.Uint32(header)))
			for i := range d {
				buf.Read(header)
				d[i] = make([]Image, int(binary.BigEndian.Uint32(header)))
				for j := range d[i] {
					buf.Read(header)
					imbytes := make([]byte, int(binary.BigEndian.Uint32(header)))
					buf.Read(imbytes)
					d[i][j] = ImageFromJPGReader(bytes.NewBuffer(imbytes))
				}
			}
			return d
		},
	}
}
