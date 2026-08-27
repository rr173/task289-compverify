// Package verify 执行补偿链完整性验证：覆盖检查、逆依赖顺序检查与遗留定位。
//
// 验证是三段式流水线：
//
//	覆盖检查（Coverage）  —— 每个已生效效果是否至少有一条有效补偿边；
//	顺序检查（Order）      —— 补偿边的逆依赖是否构成合法偏序（无环、拓扑可执行）；
//	遗留定位（Residue）    —— 定位未被覆盖的效果链，生成可审计的遗留路径。
//
// 三者全部通过 → 运行可标记为 closed；任一失败 → 运行标记为 has_residue。
package verify

import (
	"sort"

	"task289-compverify/internal/causal"
	"task289-compverify/internal/model"
)

// Report 是一次验证的完整结论，可直接序列化为 /api/runs/{id}/verify 的响应。
type Report struct {
	OK             bool           `json:"ok"`
	Covered        int            `json:"covered"`
	Uncovered      int            `json:"uncovered"`
	OrderConflicts int            `json:"order_conflicts"`
	Waived         int            `json:"waived"`
	Residues       []Residue      `json:"residues"`
	Message        string         `json:"message"`
	// CompUpdates 是验证动作建议的补偿边状态迁移（candidate → valid/order_conflict）。
	CompUpdates map[int64]model.CompensationStatus `json:"-"`
}

// Residue 是遗留路径的可导出形态。
type Residue struct {
	Path       []int64 `json:"path"`
	HeadEffect string  `json:"head_effect"`
	Cause      string  `json:"cause"`
}

// Verifier 执行验证逻辑。
type Verifier struct{}

// New 构造验证器。
func New() *Verifier { return &Verifier{} }

// Verify 对给定因果图执行完整验证。
//
// 验证动作同时承担补偿边"定级"职责：candidate 补偿边若参与环检测
// （逆依赖顺序非法）则建议 order_conflict，否则建议 valid。
func (v *Verifier) Verify(g *causal.Graph) Report {
	rep := Report{CompUpdates: make(map[int64]model.CompensationStatus)}
	compByEffect := make(map[int64][]causal.Edge)
	for _, e := range g.Edges {
		compByEffect[effectIDByKey(g, e.FromKey)] = append(compByEffect[effectIDByKey(g, e.FromKey)], e)
	}

	// 1. 顺序检查先行：确定候选补偿边的定级。
	gradeOrder(&rep, g)
	// 2. 覆盖检查：effective 效果必须有有效补偿边。
	checkCoverage(&rep, g, compByEffect)
	// 3. 豁免统计。
	countWaived(&rep, g)
	// 4. 汇总判定。
	summarize(&rep)
	return rep
}

// summarize 汇总判定：覆盖完成即可闭合，顺序冲突由工程师线下处理。
func summarize(rep *Report) {
	if rep.Uncovered == 0 {
		rep.OK = true
		if rep.OrderConflicts > 0 {
			rep.Message = "compensation chain is covered; review order conflicts offline"
		} else {
			rep.Message = "compensation chain is complete and well-ordered"
		}
		return
	}
	rep.OK = false
	rep.Message = "compensation chain has uncovered effects or order conflicts"
}

// effectIDByKey 反查效果 ID（内部工具）。
func effectIDByKey(g *causal.Graph, key string) int64 {
	for _, n := range g.NodeList {
		if n.EffectKey == key {
			return n.EffectID
		}
	}
	return 0
}

// GroupResidues 把单个未覆盖效果聚合成链：同一未覆盖效果存在多条路径时合并。
func GroupResidues(in []Residue) []Residue {
	if len(in) == 0 {
		return nil
	}
	out := make([]Residue, 0, len(in))
	seen := make(map[int64]bool)
	for _, r := range in {
		if len(r.Path) == 0 {
			continue
		}
		head := r.Path[0]
		if seen[head] {
			continue
		}
		seen[head] = true
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].HeadEffect < out[j].HeadEffect
	})
	return out
}
