package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

func decodeValue(typ reflect.Type, raw map[string]any) (reflect.Value, error) {
	if typ == nil {
		return reflect.Value{}, nil
	}
	wantPtr := typ.Kind() == reflect.Pointer
	st := typ
	if wantPtr {
		st = typ.Elem()
	}
	if st.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("expected struct, got %s", typ)
	}
	dest := reflect.New(st)
	if raw == nil {
		raw = map[string]any{}
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return reflect.Value{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest.Interface()); err != nil {
		return reflect.Value{}, err
	}
	if wantPtr {
		return dest, nil
	}
	return dest.Elem(), nil
}

func callConstructor(fn any, args ...reflect.Value) (any, error) {
	fv := reflect.ValueOf(fn)
	outs := fv.Call(args)
	if !outs[1].IsNil() {
		return nil, outs[1].Interface().(error)
	}
	if !outs[0].IsValid() || (outs[0].Kind() == reflect.Pointer && outs[0].IsNil()) {
		return outs[0].Interface(), nil
	}
	return outs[0].Interface(), nil
}
