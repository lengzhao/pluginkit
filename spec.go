package pluginkit

import (
	"fmt"
	"reflect"
)

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// Spec 描述已注册插件类型的构造函数形态。
type Spec struct {
	Kind       string
	ConfigType reflect.Type // 无 Config 参数时为 nil
	DepsType   reflect.Type // 无 Deps 参数时为 nil
	ReturnType reflect.Type
	fn         any
}

// Constructor 返回注册时的构造函数。
func (s Spec) Constructor() any {
	return s.fn
}

func parseSpec(kind string, constructor any) (Spec, error) {
	if constructor == nil {
		return Spec{}, fmt.Errorf("constructor is nil")
	}
	t := reflect.TypeOf(constructor)
	if t.Kind() != reflect.Func {
		return Spec{}, fmt.Errorf("constructor must be a function")
	}
	if t.IsVariadic() {
		return Spec{}, fmt.Errorf("constructor must not be variadic")
	}
	if t.NumOut() != 2 {
		return Spec{}, fmt.Errorf("constructor must return (T, error)")
	}
	if t.Out(1) != errorType && !t.Out(1).Implements(errorType) {
		return Spec{}, fmt.Errorf("constructor second return value must be error")
	}

	spec := Spec{
		Kind:       kind,
		ReturnType: t.Out(0),
		fn:         constructor,
	}
	switch t.NumIn() {
	case 0:
	case 1:
		if err := checkStructParam("config", t.In(0)); err != nil {
			return Spec{}, err
		}
		spec.ConfigType = t.In(0)
	case 2:
		if err := checkStructParam("config", t.In(0)); err != nil {
			return Spec{}, err
		}
		if err := checkStructParam("deps", t.In(1)); err != nil {
			return Spec{}, err
		}
		spec.ConfigType = t.In(0)
		spec.DepsType = t.In(1)
	default:
		return Spec{}, fmt.Errorf("constructor must have 0, 1, or 2 parameters")
	}
	return spec, nil
}

func checkStructParam(name string, t reflect.Type) error {
	st := t
	if st.Kind() == reflect.Pointer {
		st = st.Elem()
	}
	if st.Kind() != reflect.Struct {
		return fmt.Errorf("%s parameter must be a struct or *struct", name)
	}
	return nil
}
