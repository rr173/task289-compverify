package verify

import (
	"task289-compverify/internal/causal"
	"task289-compverify/internal/model"
)

// gradeOrder 执行顺序检查并为候选补偿边定级：
//
//   - 参与逆依赖环的候选边 → 建议 order_conflict；
//   - 其余候选边 → 建议 valid。
//
// 同时统计 OrderConflicts（环成员动作数）。
func gradeOrder(rep *Report, g *causal.Graph) {
	_, hasCycle, cycle := g.TopologicalActions()
	cycleSet := make(map[string]bool)
	for _, c := range cycle {
		cycleSet[c] = true
	}
	for _, e := range g.Edges {
		if e.Status != model.CompCandidate {
			continue
		}
		if hasCycle && cycleSet[e.Action] {
			rep.CompUpdates[e.CompID] = model.CompOrderConflict
		} else {
			rep.CompUpdates[e.CompID] = model.CompValid
		}
	}
	if hasCycle {
		rep.OrderConflicts = len(cycle)
	}
}

// checkCoverage 检查每个 effective 效果的覆盖情况：
// 存在 valid/confirmed 补偿边，或本次定级为 valid 的候选边，即视为已覆盖。
func checkCoverage(rep *Report, g *causal.Graph, compByEffect map[int64][]causal.Edge) {
	var uncovered []Residue
	for _, n := range g.NodeList {
		if n.Status != model.EffEffective {
			continue
		}
		covered := false
		for _, e := range compByEffect[n.EffectID] {
			if e.Status == model.CompValid || e.Status == model.CompConfirmed {
				covered = true
				break
			}
			if e.Status == model.CompCandidate && rep.CompUpdates[e.CompID] == model.CompValid {
				covered = true
				break
			}
		}
		if covered {
			rep.Covered++
		} else {
			rep.Uncovered++
			uncovered = append(uncovered, Residue{
				Path:       []int64{n.EffectID},
				HeadEffect: n.EffectKey,
				Cause:      "uncovered_effect",
			})
		}
	}
	rep.Residues = uncovered
}

// countWaived 统计被工程师豁免的补偿边数量。
func countWaived(rep *Report, g *causal.Graph) {
	for _, e := range g.Edges {
		if e.Status == model.CompWaived {
			rep.Waived++
		}
	}
}
