package vaas

import (
	"strconv"
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

type Parents []Parent
func (parents Parents) String() string {
	var parts []string
	for _, parent := range parents {
		if parent.Type == NodeParent {
			parts = append(parts, string(parent.Type) + strconv.Itoa(parent.NodeID))
		} else if parent.Type == SeriesParent {
			parts = append(parts, string(parent.Type) + strconv.Itoa(parent.SeriesIdx))
		}
	}
	return strings.Join(parts, ",")
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

func (node Node) GetChildren(m map[int]*Node) []Node {
	var nodes []Node
	for _, other := range m {
		for _, parent := range other.Parents {
			if parent.Type != NodeParent {
				continue
			}
			if parent.NodeID != node.ID {
				continue
			}
			nodes = append(nodes, *other)
		}
	}
	return nodes
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
