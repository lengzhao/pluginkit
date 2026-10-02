package build

import (
	"errors"
	"reflect"
)

// ValidatePlan 执行 resolve / typecheck / deps 规划，并对全图实例执行
// decode → SetDefaults → Validate 管线，但不调用插件 New。
// 配置错误聚合后一次性返回（errors.Join），便于编辑场景一次看到全部问题。
func ValidatePlan(graph map[string]any, rootID string, want reflect.Type) error {
	parsed, err := parseGraph(graph)
	if err != nil {
		return err
	}
	p, err := compileRootPlan(rootID, want, parsed)
	if err != nil {
		return err
	}
	var errs []error
	for _, n := range p.steps {
		if n.spec.ConfigType == nil {
			continue
		}
		if _, err := prepareConfig(n); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
