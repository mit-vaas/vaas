package vaas

type IntData []int
func (d IntData) IsEmpty() bool {
	for _, class := range d {
		if class != 0 {
			return false
		}
	}
	return true
}
func (d IntData) Length() int {
	return len(d)
}
func (d IntData) EnsureLength(length int) Data {
	for i := len(d); i < length; i++ {
		d = append(d, 0)
	}
	return d
}
func (d IntData) Slice(i, j int) Data {
	return d[i:j]
}
func (d IntData) Append(other Data) Data {
	other_ := other.(IntData)
	return append(d, other_...)
}
func (d IntData) Encode() []byte {
	return JsonMarshal(d)
}
func (d IntData) Type() DataType {
	return IntType
}

func init() {
	dataImpls[IntType] = DataImpl{
		New: func() Data {
			return IntData{}
		},
		Decode: func(bytes []byte) Data {
			var d IntData
			JsonUnmarshal(bytes, &d)
			return d
		},
	}
}
