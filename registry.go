package pluginkit

import (
	"fmt"
	"sync"
)

var (
	mu       sync.RWMutex
	registry = map[string]Spec{}
)

// Register 登记插件类型。kind 必须非空且不重复；constructor 必须是
// New() (T, error)、New(cfg) (T, error) 或 New(cfg, deps) (T, error)。
//
// 非法登记会 panic，便于在 init() 中尽早失败。
func Register(kind string, constructor any) {
	if err := register(kind, constructor); err != nil {
		panic(err)
	}
}

func register(kind string, constructor any) error {
	if kind == "" {
		return fmt.Errorf("plugin kind must not be empty")
	}
	spec, err := parseSpec(kind, constructor)
	if err != nil {
		return fmt.Errorf("plugin %q: %w", kind, err)
	}

	mu.Lock()
	defer mu.Unlock()
	if _, exists := registry[kind]; exists {
		return fmt.Errorf("plugin kind %q already registered", kind)
	}
	registry[kind] = spec
	return nil
}

// Lookup 按 kind 查找已注册的构造函数形态。
func Lookup(kind string) (Spec, bool) {
	mu.RLock()
	defer mu.RUnlock()
	spec, ok := registry[kind]
	return spec, ok
}

func resetRegistry() {
	mu.Lock()
	defer mu.Unlock()
	registry = map[string]Spec{}
}
