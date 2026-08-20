package pluginkit

import (
	"reflect"
	"strings"
)

// PluginDescription 描述已注册插件类型的配置字段和依赖扩展点字段。
type PluginDescription struct {
	Kind       string
	Config     []FieldDescription
	Extensions []FieldDescription
	ReturnType reflect.Type
}

// FieldDescription 描述 struct 顶层字段的配置名、Go 名、类型和单值/多值/可选规则。
type FieldDescription struct {
	Name     string
	GoName   string
	Type     reflect.Type
	List     bool
	Optional bool
}

// Describe 按 kind 返回已注册插件类型的元信息。
// 未注册时返回 false，不报错。
func Describe(kind string) (PluginDescription, bool) {
	spec, ok := Lookup(kind)
	if !ok {
		return PluginDescription{}, false
	}
	return PluginDescription{
		Kind:       spec.Kind,
		Config:     describeStructFields(spec.ConfigType),
		Extensions: describeStructFields(spec.DepsType),
		ReturnType: spec.ReturnType,
	}, true
}

// Template 返回可填入配置的骨架，格式与 build 接受的 PluginUse 一致。
// init() 注册后即可调用，不构造实例。config 字段为零值占位；deps 使用 use: "" 占位；
// 可选 deps 会省略。
func (d PluginDescription) Template() map[string]any {
	tmpl := map[string]any{"use": d.Kind}
	if cfg := templateConfig(d.Config); len(cfg) > 0 {
		tmpl["config"] = cfg
	}
	if deps := templateDeps(d.Extensions); len(deps) > 0 {
		tmpl["deps"] = deps
	}
	return tmpl
}

func describeStructFields(typ reflect.Type) []FieldDescription {
	if typ == nil {
		return nil
	}
	st := typ
	if st.Kind() == reflect.Pointer {
		st = st.Elem()
	}
	if st.Kind() != reflect.Struct {
		return nil
	}

	var fields []FieldDescription
	for i := 0; i < st.NumField(); i++ {
		sf := st.Field(i)
		if sf.PkgPath != "" {
			continue
		}
		name, optional, skip := jsonFieldName(sf)
		if skip {
			continue
		}
		ft := sf.Type
		list := false
		if ft.Kind() == reflect.Slice {
			list = true
			ft = ft.Elem()
		}
		fields = append(fields, FieldDescription{
			Name:     name,
			GoName:   sf.Name,
			Type:     ft,
			List:     list,
			Optional: optional,
		})
	}
	return fields
}

func jsonFieldName(sf reflect.StructField) (name string, optional bool, skip bool) {
	tag := sf.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}
	name, opts, _ := strings.Cut(tag, ",")
	if name == "" {
		name = sf.Name
	}
	return name, strings.Contains(opts, "omitempty"), false
}

func templateConfig(fields []FieldDescription) map[string]any {
	if len(fields) == 0 {
		return nil
	}
	cfg := make(map[string]any, len(fields))
	for _, field := range fields {
		cfg[field.Name] = templateFieldValue(field)
	}
	return cfg
}

func templateDeps(fields []FieldDescription) map[string]any {
	deps := make(map[string]any)
	for _, field := range fields {
		if field.Optional {
			continue
		}
		deps[field.Name] = templateDepValue(field)
	}
	if len(deps) == 0 {
		return nil
	}
	return deps
}

func templateFieldValue(field FieldDescription) any {
	if field.List {
		return []any{zeroValueForType(field.Type)}
	}
	return zeroValueForType(field.Type)
}

func templateDepValue(field FieldDescription) any {
	placeholder := map[string]any{"use": ""}
	if field.List {
		return []any{placeholder}
	}
	return placeholder
}

func zeroValueForType(t reflect.Type) any {
	if t == nil {
		return nil
	}
	switch t.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Struct, reflect.Map, reflect.Slice, reflect.Array:
		return nil
	default:
		return reflect.Zero(t).Interface()
	}
}
