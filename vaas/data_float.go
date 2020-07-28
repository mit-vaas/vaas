package vaas

type FloatData []float64
func (d FloatData) IsEmpty() bool {
	for _, class := range d {
		if class != 0 {
			return false
		}
	}
	return true
}
func (d FloatData) Length() int {
	return len(d)
}
func (d FloatData) EnsureLength(length int) Data {
	for i := len(d); i < length; i++ {
		d = append(d, 0)
	}
	return d
}
func (d FloatData) Slice(i, j int) Data {
	return d[i:j]
}
func (d FloatData) Append(other Data) Data {
	other_ := other.(FloatData)
	return append(d, other_...)
}
func (d FloatData) Encode() []byte {
	return JsonMarshal(d)
}
func (d FloatData) Type() DataType {
	return FloatType
}

func init() {
	dataImpls[FloatType] = DataImpl{
		New: func() Data {
			return FloatData{}
		},
		Decode: func(bytes []byte) Data {
			var d FloatData
			JsonUnmarshal(bytes, &d)
			return d
		},
	}
}
