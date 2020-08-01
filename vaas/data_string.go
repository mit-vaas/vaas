package vaas

type StringData []string
func (d StringData) IsEmpty() bool {
	for _, str := range d {
		if str != "" {
			return false
		}
	}
	return true
}
func (d StringData) Length() int {
	return len(d)
}
func (d StringData) EnsureLength(length int) Data {
	for i := len(d); i < length; i++ {
		d = append(d, "")
	}
	return d
}
func (d StringData) Slice(i, j int) Data {
	return d[i:j]
}
func (d StringData) Append(other Data) Data {
	other_ := other.(StringData)
	return append(d, other_...)
}
func (d StringData) Encode() []byte {
	return JsonMarshal(d)
}
func (d StringData) Type() DataType {
	return StringType
}

func init() {
	dataImpls[StringType] = DataImpl{
		New: func() Data {
			return StringData{}
		},
		Decode: func(bytes []byte) Data {
			var d StringData
			JsonUnmarshal(bytes, &d)
			return d
		},
	}
}
