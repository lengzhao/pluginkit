package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/lengzhao/pluginkit"
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

// prepareConfig 是单节点的 decode → SetDefaults → Validate 管线，
// 被 construct（首错即停）和 ValidatePlan（聚合错误）共同复用，
// 保证「编辑器里看到的错误」和「Build 时的错误」完全一致。
// 返回应传给构造函数的配置值；失败时返回带阶段的装配错误。
func prepareConfig(n *node) (reflect.Value, error) {
	val, err := decodeValue(n.spec.ConfigType, n.use.Config)
	if err != nil {
		return val, assembleErr(n.field, n.use.Use, n.use.ID, StageDecode, err)
	}
	val, err = applyConfigHooks(val)
	if err != nil {
		return val, assembleErr(n.field, n.use.Use, n.use.ID, StageValidate, err)
	}
	return val, nil
}

// applyConfigHooks 在 decode 之后、New 之前执行配置钩子：
// 先 SetDefaults（若实现 pluginkit.Defaulter），再 Validate（若实现 pluginkit.Validator）。
// 值类型 Config 会先拷贝再取地址，指针接收者上的方法对值/指针两种 ConfigType 都生效；
// SetDefaults 的修改作用于拷贝，最终通过返回值传回，不会原地修改入参。
// 返回应传给构造函数的配置值。
func applyConfigHooks(val reflect.Value) (reflect.Value, error) {
	ptr := val
	if ptr.Kind() != reflect.Pointer {
		ptr = reflect.New(val.Type())
		ptr.Elem().Set(val)
	}
	if d, ok := ptr.Interface().(pluginkit.Defaulter); ok {
		d.SetDefaults()
	}
	if v, ok := ptr.Interface().(pluginkit.Validator); ok {
		if err := v.Validate(); err != nil {
			return val, err
		}
	}
	if val.Kind() == reflect.Pointer {
		return ptr, nil
	}
	return ptr.Elem(), nil
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
