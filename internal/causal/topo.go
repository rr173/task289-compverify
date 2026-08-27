package causal

import "sort"

// TopologicalActions 返回补偿动作的逆依赖拓扑序；同时报告环检测结果。
//
// 返回 (有序动作列表, 是否有环, 环成员)。拓扑序满足：若动作 A 依赖动作 B
// （A.DepsOn 包含 B），则 B 必须先于 A 执行。存在环时返回 nil 顺序，
// cycle 为残余 indeg>0 的动作集合（参与环或依赖环）。
func (g *Graph) TopologicalActions() ([]string, bool, []string) {
	adj := make(map[string][]string)
	indeg := make(map[string]int)
	actionNames := make(map[string]struct{})
	for _, e := range g.Edges {
		actionNames[e.Action] = struct{}{}
	}
	for _, e := range g.Edges {
		for _, d := range e.DepsOn {
			if _, exists := actionNames[d]; !exists {
				continue // 依赖外部动作，不参与内部拓扑
			}
			adj[d] = append(adj[d], e.Action)
			indeg[e.Action]++
		}
		if _, ok := indeg[e.Action]; !ok {
			indeg[e.Action] = 0
		}
	}
	// Kahn 拓扑排序。
	queue := make([]string, 0)
	for a := range actionNames {
		if indeg[a] == 0 {
			queue = append(queue, a)
		}
	}
	sort.Strings(queue)
	order := make([]string, 0, len(actionNames))
	visited := 0
	for len(queue) > 0 {
		a := queue[0]
		queue = queue[1:]
		order = append(order, a)
		visited++
		for _, next := range adj[a] {
			indeg[next]--
			if indeg[next] == 0 {
				queue = append(queue, next)
			}
		}
		sort.Strings(queue)
	}
	if visited != len(actionNames) {
		// 检测环：找残余 indeg>0 的动作。
		var cycle []string
		for a := range actionNames {
			if indeg[a] > 0 {
				cycle = append(cycle, a)
			}
		}
		sort.Strings(cycle)
		return nil, true, cycle
	}
	return order, false, nil
}
