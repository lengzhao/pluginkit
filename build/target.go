package build

import (
	"fmt"
	"reflect"
	"strings"
)

type targetField struct {
	jsonName  string
	index     int
	slice     bool
	elemType  reflect.Type
	optional  bool
	structFld reflect.StructField
}

func inspectTarget(target any) (reflect.Value, []targetField, error) {
	if target == nil {
		return reflect.Value{}, nil, fmt.Errorf("target is nil")
	}
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return reflect.Value{}, nil, fmt.Errorf("target must be a non-nil pointer to struct")
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, nil, fmt.Errorf("target must be a pointer to struct")
	}

	t := v.Type()
	var fields []targetField
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" {
			continue
		}
		tag := sf.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if name == "" {
			name = sf.Name
		}
		ft := sf.Type
		tf := targetField{
			jsonName:  name,
			index:     i,
			optional:  strings.Contains(opts, "omitempty"),
			structFld: sf,
		}
		if ft.Kind() == reflect.Slice {
			tf.slice = true
			tf.elemType = ft.Elem()
		} else {
			tf.elemType = ft
		}
		fields = append(fields, tf)
	}
	return v, fields, nil
}
