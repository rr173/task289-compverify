// Package causal 从事件流重建事务的因果图：效果节点 + 补偿边 + 重试代次。
//
// 因果图是补偿链验证的中枢数据结构。它回答两个问题：
//
//  1. 哪些效果已被补偿边覆盖（覆盖关系）；
//  2. 补偿边之间的逆依赖顺序是否构成合法偏序（顺序关系）。
//
// 图以内存形式构建，节点 = 效果（effect_id），有向边 = 补偿边
// （effect --compensated-by--> action），并附带代次（generation）信息，
// 用于诊断重试引起的重复效果与代次冲突。
package causal

import (
	"sort"
	"strings"

	"task289-compverify/internal/model"
)

// Node 是因果图中的一个效果节点。
type Node struct {
	EffectID  int64            `json:"effect_id"`
	EffectKey string           `json:"effect_key"`
	StepName  string           `json:"step_name"`
	Status    model.EffectStatus `json:"status"`
	Gen       int              `json:"gen"`
	OutEdges  []string         `json:"out_edges"` // 该效果引出的补偿动作
}

// Edge 是补偿边。
type Edge struct {
	CompID  int64  `json:"comp_id"`
	FromKey string `json:"from_key"` // 被补偿的效果指纹
	Action  string `json:"action"`   // 补偿动作名
	DepsOn  []string `json:"deps_on"`
	Status  model.CompensationStatus `json:"status"`
	Gen     int    `json:"gen"`
}

// Graph 是一次运行的整体因果图。
type Graph struct {
	RunID    int64            `json:"run_id"`
	Nodes    map[int64]*Node  `json:"-"`
	Edges    []Edge           `json:"edges"`
	NodeList []Node           `json:"nodes"`
	MaxGen   int              `json:"max_gen"`
}

// Builder 基于 store 数据构建因果图。
type Builder struct {
	effectsByKey map[string]model.Effect
	stepNames    map[int64]string
}

// NewBuilder 预加载步骤名映射，供构建时还原步骤名。
func NewBuilder(effects []model.Effect, steps map[int64]string) *Builder {
	b := &Builder{
		effectsByKey: make(map[string]model.Effect, len(effects)),
		stepNames:    steps,
	}
	for _, e := range effects {
		b.effectsByKey[e.EffectKey] = e
	}
	return b
}

// Build 由效果列表与补偿边列表构建因果图。
func (b *Builder) Build(runID int64, effects []model.Effect, comps []model.Compensation) *Graph {
	g := &Graph{
		RunID:  runID,
		Nodes:  make(map[int64]*Node, len(effects)),
		Edges:  make([]Edge, 0, len(comps)),
	}
	for _, e := range effects {
		n := &Node{
			EffectID:  e.ID,
			EffectKey: e.EffectKey,
			StepName:  b.stepNames[e.StepID],
			Status:    e.Status,
			Gen:       e.Gen,
			OutEdges:  make([]string, 0),
		}
		if n.StepName == "" {
			n.StepName = "unknown"
		}
		g.Nodes[e.ID] = n
		if e.Gen > g.MaxGen {
			g.MaxGen = e.Gen
		}
	}
	for _, c := range comps {
		e, ok := b.effectsByKey[effectKeyFor(c, effects)]
		fromKey := ""
		if ok {
			fromKey = e.EffectKey
			if n, found := g.Nodes[e.ID]; found {
				n.OutEdges = append(n.OutEdges, c.Action)
			}
		} else {
			fromKey = c.Action // 无法还原时退回动作名
		}
		g.Edges = append(g.Edges, Edge{
			CompID:  c.ID,
			FromKey: fromKey,
			Action:  c.Action,
			DepsOn:  append([]string(nil), c.DepsOn...),
			Status:  c.Status,
			Gen:     c.Gen,
		})
	}
	// 输出稳定的节点顺序（按 effect_id）。
	g.NodeList = make([]Node, 0, len(g.Nodes))
	ids := make([]int64, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		g.NodeList = append(g.NodeList, *g.Nodes[id])
	}
	return g
}

// effectKeyFor 从效果列表反查效果指纹；找不到返回空串。
func effectKeyFor(c model.Compensation, effects []model.Effect) string {
	for _, e := range effects {
		if e.ID == c.EffectID {
			return e.EffectKey
		}
	}
	return ""
}


// DescribeGraph 输出图的人类可读摘要（用于快照 summary）。
func (g *Graph) DescribeGraph() string {
	var sb strings.Builder
	sb.WriteString("effects=")
	sb.WriteString(itoaSafe(len(g.NodeList)))
	sb.WriteString(",edges=")
	sb.WriteString(itoaSafe(len(g.Edges)))
	sb.WriteString(",maxgen=")
	sb.WriteString(itoaSafe(g.MaxGen))
	return sb.String()
}

// NodeListToEffects 把图节点转为模型效果列表（供指纹计算）。
func (g *Graph) NodeListToEffects() []model.Effect {
	out := make([]model.Effect, 0, len(g.NodeList))
	for _, n := range g.NodeList {
		out = append(out, model.Effect{
			ID:        n.EffectID,
			EffectKey: n.EffectKey,
			Status:    n.Status,
			Gen:       n.Gen,
		})
	}
	return out
}

// EdgesToCompensations 把图边转为模型补偿列表（供指纹计算）。
func (g *Graph) EdgesToCompensations() []model.Compensation {
	out := make([]model.Compensation, 0, len(g.Edges))
	for _, e := range g.Edges {
		out = append(out, model.Compensation{
			ID:     e.CompID,
			Action: e.Action,
			Status: e.Status,
			Gen:    e.Gen,
		})
	}
	return out
}

// ResiduesToModel 把图转化为空遗留列表（供指纹计算；遗留已由 diagnose 落库）。
func (g *Graph) ResiduesToModel() []model.Residue {
	return nil
}

func itoaSafe(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
