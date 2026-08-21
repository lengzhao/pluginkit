package pluginkit

import (
	"reflect"
	"sort"
)

// ListKinds 返回已注册插件 kind，按字典序排序。
func ListKinds() []string {
	mu.RLock()
	defer mu.RUnlock()
	kinds := make([]string, 0, len(registry))
	for kind := range registry {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

// CompatibleKinds 返回构造返回值静态上可满足 want 的插件 kind。
// 精确匹配扩展点类型的 kind 排在前面，其次是具体类型实现，最后是接口返回的宽松匹配。
func CompatibleKinds(want reflect.Type) []string {
	if want == nil {
		return nil
	}
	mu.RLock()
	defer mu.RUnlock()
	type rankedKind struct {
		kind string
		rank int
	}
	var kinds []rankedKind
	for kind, spec := range registry {
		if ok, rank := returnTypeMatchRank(want, spec.ReturnType); ok {
			kinds = append(kinds, rankedKind{kind: kind, rank: rank})
		}
	}
	sort.Slice(kinds, func(i, j int) bool {
		if kinds[i].rank != kinds[j].rank {
			return kinds[i].rank < kinds[j].rank
		}
		return kinds[i].kind < kinds[j].kind
	})
	out := make([]string, len(kinds))
	for i, item := range kinds {
		out[i] = item.kind
	}
	return out
}

// FormatType 把 reflect.Type 格式化为可读字符串，供 UI 展示。
func FormatType(t reflect.Type) string {
	if t == nil {
		return ""
	}
	return t.String()
}

// returnTypeMatchRank 返回是否兼容，以及匹配优先级（越小越靠前）。
func returnTypeMatchRank(want, ret reflect.Type) (bool, int) {
	if ret == nil || want == nil {
		return false, 0
	}
	if ret == want {
		return true, 0
	}
	if ret.Kind() != reflect.Interface && want.Kind() == reflect.Interface && ret.Implements(want) {
		return true, 1
	}
	if want.Kind() != reflect.Interface && ret.AssignableTo(want) {
		return true, 1
	}
	if ret.Kind() == reflect.Interface {
		return true, 2
	}
	return false, 0
}
