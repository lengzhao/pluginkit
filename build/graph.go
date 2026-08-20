package build

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/pluginkit/config"
)

type node struct {
	index  int
	field  string
	list   bool
	target targetField
	want   reflect.Type
	use    config.PluginUse
	spec   pluginkit.Spec
}

func topoSort(nodes []*node) ([]*node, error) {
	byID := make(map[string]*node, len(nodes))
	for _, n := range nodes {
		byID[n.use.ID] = n
	}

	indeg := make(map[string]int, len(nodes))
	outgoing := make(map[string][]string, len(nodes))
	for _, n := range nodes {
		indeg[n.use.ID] = 0
	}
	for _, n := range nodes {
		ids, err := collectDepIDs(n.use.Deps)
		if err != nil {
			return nil, assembleErr(n.field, n.use.Use, n.use.ID, StageDeps, err)
		}
		seen := map[string]bool{}
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			if _, ok := byID[id]; !ok {
				return nil, assembleErr(n.field, n.use.Use, n.use.ID, StageDeps, fmt.Errorf("unknown instance %q", id))
			}
			outgoing[id] = append(outgoing[id], n.use.ID)
			indeg[n.use.ID]++
		}
	}

	var ready []string
	for _, n := range nodes {
		if indeg[n.use.ID] == 0 {
			ready = append(ready, n.use.ID)
		}
	}
	sortReady := func() {
		sort.Slice(ready, func(i, j int) bool {
			return byID[ready[i]].index < byID[ready[j]].index
		})
	}
	sortReady()

	order := make([]*node, 0, len(nodes))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, byID[id])
		for _, nxt := range outgoing[id] {
			indeg[nxt]--
			if indeg[nxt] == 0 {
				ready = append(ready, nxt)
			}
		}
		sortReady()
	}
	if len(order) != len(nodes) {
		var leftover []string
		for _, n := range nodes {
			if indeg[n.use.ID] > 0 {
				leftover = append(leftover, n.use.ID)
			}
		}
		sort.Strings(leftover)
		return nil, assembleErr("", "", "", StageDeps, fmt.Errorf("dependency cycle among %v", leftover))
	}
	return order, nil
}
