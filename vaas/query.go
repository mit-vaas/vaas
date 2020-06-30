package vaas

import (
	"strings"
)

type ParentType string
const (
	NodeParent ParentType = "n"
	SeriesParent = "s"
)
type Parent struct {
	Spec string
	Type ParentType
	NodeID int
	SeriesIdx int
}

func ParseParents(str string) []Parent {
	parents := []Parent{}
	if str == "" {
		return parents
	}
	for _, part := range strings.Split(str, ",") {
		var parent Parent
		parent.Spec = part
		parent.Type = ParentType(part[0:1])
		if parent.Type == NodeParent {
			parent.NodeID = ParseInt(part[1:])
		} else if parent.Type == SeriesParent {
			parent.SeriesIdx = ParseInt(part[1:])
		}
		parents = append(parents, parent)
	}
	return parents
}

type Node struct {
	ID int
	Name string
	ParentTypes []DataType
	Parents []Parent
	Type string
	DataType DataType
	Code string
	QueryID int
}

type VNode struct {
	ID int

	Vector []Series
	Node Node
	Series *Series
}

// Metadata for rendering query and its nodes on the front-end query editor.
type QueryRenderMeta map[string][2]int

type Query struct {
	ID int
	Name string
	OutputsStr string
	SelectorID *int
	RenderMeta QueryRenderMeta

	Outputs [][]Parent
	Selector *Node

	Nodes map[int]*Node
}
