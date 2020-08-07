package builtins

import (
	"../vaas"

	"encoding/json"
	"fmt"
)

type BoolExprConfig struct {}

type BoolExpr struct {
	node vaas.Node
	cfg BoolExprConfig
	stats *vaas.StatsHolder
}

func NewBoolExpr(node vaas.Node) vaas.Executor {
	var cfg BoolExprConfig
	err := json.Unmarshal([]byte(node.Code), &cfg)
	if err != nil {
		return vaas.ErrorExecutor{node.DataType, fmt.Errorf("error decoding node configuration: %v", err)}
	}
	return BoolExpr{
		node: node,
		cfg: cfg,
		stats: new(vaas.StatsHolder),
	}
}

func (m BoolExpr) Run(ctx vaas.ExecContext) vaas.DataBuffer {
	buf := vaas.NewSimpleBuffer(vaas.IntType)
	buf.SetMeta(1)

	go func() {
		for _, parent := range m.node.Parents {
			rd, err := GetParent(ctx, parent)
			if err != nil {
				buf.Error(err)
				return
			}
			data, err := rd.Read(ctx.Slice.Length())
			if err != nil {
				buf.Error(err)
				return
			}
			if data.IsEmpty() {
				buf.Write(vaas.IntData{0}.EnsureLength(ctx.Slice.Length()))
				buf.Close()
				return
			}
		}
		buf.Write(vaas.IntData{1}.EnsureLength(ctx.Slice.Length()))
		buf.Close()
	}()

	return buf
}

func (m BoolExpr) Close() {}

func (m BoolExpr) Stats() vaas.StatsSample {
	return m.stats.Get()
}

func init() {
	vaas.Executors["bool-expr"] = vaas.ExecutorMeta{
		New: NewBoolExpr,
	}
}
