package build

// TypedInstance 是已构造实例及其图 id 和 kind。
type TypedInstance[T any] struct {
	ID    string
	Use   string
	Value T
}

// Collect 返回 result 中所有值可断言为 T 的实例。
// result 为 nil 时返回 nil。
func Collect[T any](result *Result) []T {
	if result == nil {
		return nil
	}
	out := make([]T, 0)
	for _, inst := range result.Instances {
		value, ok := inst.Value.(T)
		if !ok {
			continue
		}
		out = append(out, value)
	}
	return out
}

// CollectInstances 返回所有匹配实例及其 id 和 kind。
// result 为 nil 时返回 nil。
func CollectInstances[T any](result *Result) []TypedInstance[T] {
	if result == nil {
		return nil
	}
	out := make([]TypedInstance[T], 0)
	for _, inst := range result.Instances {
		value, ok := inst.Value.(T)
		if !ok {
			continue
		}
		out = append(out, TypedInstance[T]{
			ID:    inst.ID,
			Use:   inst.Use,
			Value: value,
		})
	}
	return out
}
