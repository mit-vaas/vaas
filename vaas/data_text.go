package vaas

type RichText struct {
	Text string
	X int
	Y int
}

type TextData []RichText
func (d TextData) IsEmpty() bool {
	for _, text := range d {
		if text.Text != "" {
			return false
		}
	}
	return true
}
func (d TextData) Length() int {
	return len(d)
}
func (d TextData) EnsureLength(length int) Data {
	for i := len(d); i < length; i++ {
		d = append(d, RichText{})
	}
	return d
}
func (d TextData) Slice(i, j int) Data {
	return d[i:j]
}
func (d TextData) Append(other Data) Data {
	other_ := other.(TextData)
	return append(d, other_...)
}
func (d TextData) Encode() []byte {
	return JsonMarshal(d)
}
func (d TextData) Type() DataType {
	return TextType
}

func init() {
	dataImpls[TextType] = DataImpl{
		New: func() Data {
			return TextData{}
		},
		Decode: func(bytes []byte) Data {
			var d TextData
			JsonUnmarshal(bytes, &d)
			return d
		},
	}
}
