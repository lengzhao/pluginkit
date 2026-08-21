package build

import "reflect"

// ValidatePlan 只执行 resolve / typecheck / deps 规划，不调用插件 New。
func ValidatePlan(graph map[string]any, rootID string, want reflect.Type) error {
	parsed, err := parseGraph(graph)
	if err != nil {
		return err
	}
	_, err = compileRootPlan(rootID, want, parsed)
	return err
}
