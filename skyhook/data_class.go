package skyhook

type ClassData []int
func (d ClassData) IsEmpty() bool {
	for _, class := range d {
		if class != 0 {
			return false
		}
	}
	return true
}
func (d ClassData) Length() int {
	return len(d)
}
func (d ClassData) EnsureLength(length int) Data {
	for i := len(d); i < length; i++ {
		d = append(d, 0)
	}
	return d
}
func (d ClassData) Slice(i, j int) Data {
	return d[i:j]
}
func (d ClassData) Append(other Data) Data {
	other_ := other.(ClassData)
	return append(d, other_...)
}
func (d ClassData) Encode() []byte {
	return JsonMarshal(d)
}
func (d ClassData) Type() DataType {
	return ClassType
}

func init() {
	dataImpls[ClassType] = DataImpl{
		New: func() Data {
			return ClassData{}
		},
		Decode: func(bytes []byte) Data {
			var d ClassData
			JsonUnmarshal(bytes, &d)
			return d
		},
	}
}
