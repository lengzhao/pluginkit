package build

import (
	"fmt"
	"reflect"
	"strings"
)

func collectDepIDs(deps map[string]any) ([]string, error) {
	if len(deps) == 0 {
		return nil, nil
	}
	var ids []string
	for key, val := range deps {
		got, err := depValues(val)
		if err != nil {
			return nil, fmt.Errorf("deps.%s: %w", key, err)
		}
		ids = append(ids, got...)
	}
	return ids, nil
}

func depValues(val any) ([]string, error) {
	switch v := val.(type) {
	case string:
		if v == "" {
			return nil, fmt.Errorf("empty instance id")
		}
		return []string{v}, nil
	case []string:
		for _, id := range v {
			if id == "" {
				return nil, fmt.Errorf("empty instance id")
			}
		}
		return append([]string(nil), v...), nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok || s == "" {
				return nil, fmt.Errorf("instance id must be a non-empty string")
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("must be a string or list of strings, got %T", val)
	}
}

func injectDeps(specType reflect.Type, raw map[string]any, instances map[string]any) (reflect.Value, error) {
	val, err := decodeValue(specType, map[string]any{})
	if err != nil {
		return reflect.Value{}, err
	}
	structVal := val
	if structVal.Kind() == reflect.Pointer {
		structVal = structVal.Elem()
	}
	st := structVal.Type()
	seen := map[string]bool{}
	for i := 0; i < st.NumField(); i++ {
		sf := st.Field(i)
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
		optional := strings.Contains(opts, "omitempty")
		seen[name] = true
		rawVal, ok := raw[name]
		if !ok {
			if optional {
				continue
			}
			return reflect.Value{}, fmt.Errorf("missing required dep %q", name)
		}
		ids, err := depValues(rawVal)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("dep %q: %w", name, err)
		}
		fv := structVal.Field(i)
		if fv.Kind() == reflect.Slice {
			slice := reflect.MakeSlice(fv.Type(), 0, len(ids))
			for _, id := range ids {
				inst, ok := instances[id]
				if !ok {
					return reflect.Value{}, fmt.Errorf("unknown instance %q", id)
				}
				iv := reflect.ValueOf(inst)
				elem := reflect.New(fv.Type().Elem()).Elem()
				if err := assignValue(elem, iv, id); err != nil {
					return reflect.Value{}, err
				}
				slice = reflect.Append(slice, elem)
			}
			fv.Set(slice)
			continue
		}
		if len(ids) != 1 {
			return reflect.Value{}, fmt.Errorf("dep %q is not a slice, need exactly one instance id", name)
		}
		inst, ok := instances[ids[0]]
		if !ok {
			return reflect.Value{}, fmt.Errorf("unknown instance %q", ids[0])
		}
		iv := reflect.ValueOf(inst)
		if err := assignValue(fv, iv, ids[0]); err != nil {
			return reflect.Value{}, err
		}
	}
	for key := range raw {
		if !seen[key] {
			return reflect.Value{}, fmt.Errorf("unknown dep %q", key)
		}
	}
	return val, nil
}

func assignValue(fv, iv reflect.Value, id string) error {
	want := fv.Type()
	if want.Kind() == reflect.Interface {
		if iv.Type().Implements(want) || (iv.Kind() == reflect.Pointer && iv.Type().Implements(want)) {
			fv.Set(iv)
			return nil
		}
		if iv.CanAddr() && iv.Addr().Type().Implements(want) {
			fv.Set(iv.Addr())
			return nil
		}
		return fmt.Errorf("instance %q (%s) does not implement %s", id, iv.Type(), want)
	}
	if iv.Type().AssignableTo(want) {
		fv.Set(iv)
		return nil
	}
	return fmt.Errorf("instance %q (%s) is not assignable to %s", id, iv.Type(), want)
}
