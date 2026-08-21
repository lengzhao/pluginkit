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
// 规则与 build 包静态类型检查一致：接口返回类型在静态阶段视为兼容。
func CompatibleKinds(want reflect.Type) []string {
	if want == nil {
		return nil
	}
	mu.RLock()
	defer mu.RUnlock()
	var kinds []string
	for kind, spec := range registry {
		if compatibleReturnType(want, spec.ReturnType) {
			kinds = append(kinds, kind)
		}
	}
	sort.Strings(kinds)
	return kinds
}

// FormatType 把 reflect.Type 格式化为可读字符串，供 UI 展示。
func FormatType(t reflect.Type) string {
	if t == nil {
		return ""
	}
	return t.String()
}

func compatibleReturnType(want, ret reflect.Type) bool {
	if ret == nil {
		return false
	}
	if ret.Kind() == reflect.Interface {
		return true
	}
	if want.Kind() == reflect.Interface {
		return ret.Implements(want)
	}
	return ret.AssignableTo(want)
}
