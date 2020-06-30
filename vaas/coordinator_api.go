package vaas

type AddOutputItemRequest struct {
	Node Node
	Vector []Series
	Slice Slice
	Format string
	Freq int
	Dims [2]int
}
