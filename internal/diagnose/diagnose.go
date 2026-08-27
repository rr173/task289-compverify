// Package diagnose 定位遗留状态：把未覆盖效果扩展为完整的效果依赖链。
//
// 覆盖检查只标记"哪个效果未被补偿"，而诊断要回答"这条副作用是如何产生的、
// 依赖哪些前置效果"——即生成可审计的遗留路径（residue path）。路径按
// 效果创建顺序展开，链头即未被补偿的效果，供工程师确认外部不可补偿步骤。
package diagnose

import (
	"sort"

	"task289-compverify/internal/causal"
	"task289-compverify/internal/model"
	"task289-compverify/internal/verify"
)

// Service 定位并持久化遗留路径。
type Service struct{}

// New 构造诊断服务。
func New() *Service { return &Service{} }

// Locate 对验证报告中的未覆盖效果展开为完整路径，返回模型化的遗留记录。
// g 用于解析效果顺序与依赖，rep 提供未覆盖清单。
func (s *Service) Locate(g *causal.Graph, rep verify.Report) []model.Residue {
	// 建立 effect_id → 创建序（用 NodeList 顺序近似）。
	order := make(map[int64]int)
	for i, n := range g.NodeList {
		order[n.EffectID] = i
	}
	// 按效果 ID 收集未覆盖者。
	var missing []int64
	for _, r := range rep.Residues {
		if len(r.Path) > 0 {
			missing = append(missing, r.Path[0])
		}
	}
	sort.Slice(missing, func(i, j int) bool { return order[missing[i]] < order[missing[j]] })

	var out []model.Residue
	for _, id := range missing {
		// 展开路径：取创建序 ≤ 该效果的兄弟效果作为前置依赖（简化模型：
		// 同一步骤的早期效果视为前置）。
		var path []int64
		var head string
		for _, n := range g.NodeList {
			if n.EffectID == id {
				head = n.EffectKey
				path = append(path, n.EffectID)
				break
			}
			if order[n.EffectID] < order[id] && sameStep(g, n.EffectID, id) {
				path = append(path, n.EffectID)
			}
		}
		out = append(out, model.Residue{
			Path:       path,
			HeadEffect: head,
			Cause:      "uncovered_effect",
		})
	}
	return out
}

// sameStep 判断两个效果是否属于同一步骤（通过节点 StepName 比较）。
func sameStep(g *causal.Graph, a, b int64) bool {
	var na, nb string
	for _, n := range g.NodeList {
		if n.EffectID == a {
			na = n.StepName
		}
		if n.EffectID == b {
			nb = n.StepName
		}
	}
	return na != "" && na == nb
}
