package build

import (
	"context"
	"fmt"
	"reflect"

	"github.com/lengzhao/pluginkit/config"
)

// Instance 是一次构造成功的插件实例。
type Instance struct {
	ID    string
	Use   string
	Value any
}

// Result 保存本次 Build 构造出的全部实例。
type Result struct {
	Instances []Instance
}

// GetByID 按实例 id 取出指定类型的值。
func GetByID[T any](result *Result, id string) (T, bool) {
	var zero T
	if result == nil {
		return zero, false
	}
	for _, inst := range result.Instances {
		if inst.ID != id {
			continue
		}
		v, ok := inst.Value.(T)
		return v, ok
	}
	return zero, false
}

// RequireByID 按实例 id 取出指定类型的值，并在不存在或类型不匹配时返回装配错误。
//
// field 用于错误信息，例如 "workflow.steps[2]"。
func RequireByID[T any](result *Result, field, id string) (T, error) {
	var zero T
	if result == nil {
		return zero, assembleErr(field, "", id, StageResolve, fmt.Errorf("result is nil"))
	}
	want := reflect.TypeOf((*T)(nil)).Elem()
	for _, inst := range result.Instances {
		if inst.ID != id {
			continue
		}
		use := config.PluginUse{ID: inst.ID, Use: inst.Use}
		if err := checkRuntimeType(want, inst.Value, use); err != nil {
			return zero, assembleErr(field, inst.Use, inst.ID, StageTypeCheck, err)
		}
		value, ok := inst.Value.(T)
		if !ok {
			return zero, assembleErr(field, inst.Use, inst.ID, StageTypeCheck, fmt.Errorf("instance %q cannot be used as requested type", id))
		}
		return value, nil
	}
	return zero, assembleErr(field, "", id, StageResolve, fmt.Errorf("unknown instance id %q", id))
}

// Build 从实例图中按 rootID 构造指定类型的根实例。
// deps 可以引用已有实例 id，也可以内联私有插件对象；内联实例会按路径生成 id。
func Build[T any](ctx context.Context, graph map[string]any, rootID string) (T, *Result, error) {
	var zero T
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return zero, nil, err
	}

	parsed, err := parseGraph(graph)
	if err != nil {
		return zero, nil, err
	}
	want := reflect.TypeOf((*T)(nil)).Elem()
	p, err := compileRootPlan(rootID, want, parsed)
	if err != nil {
		return zero, nil, err
	}
	exec, err := executePlan(ctx, p)
	if err != nil {
		return zero, nil, err
	}
	root, err := RequireByID[T](exec.result, rootID, rootID)
	if err != nil {
		return zero, nil, err
	}
	return root, exec.result, nil
}

// BuildInto 按 plugins 配置构造实例并写入 target。
// target 必须是指向 struct 的指针；struct 字段的 json tag、类型与是否为 slice 构成扩展点。
func BuildInto(ctx context.Context, plugins map[string]any, target any) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tv, fields, err := inspectTarget(target)
	if err != nil {
		return nil, assembleErr("", "", "", StageResolve, err)
	}

	parsed, err := config.Parse(plugins)
	if err != nil {
		return nil, assembleErr("", "", "", StageResolve, err)
	}

	p, err := compilePlan(parsed, fields)
	if err != nil {
		return nil, err
	}
	exec, err := executePlan(ctx, p)
	if err != nil {
		return nil, err
	}
	if err := assignTarget(tv, p, exec.instances); err != nil {
		return nil, err
	}
	return exec.result, nil
}

func firstUse(pf config.Field) (id, use string) {
	if len(pf.Uses) == 0 {
		return "", ""
	}
	return pf.Uses[0].ID, pf.Uses[0].Use
}

func construct(n *node, constructed map[string]any) (any, error) {
	spec := n.spec
	fn := spec.Constructor()
	var args []reflect.Value

	if spec.ConfigType != nil {
		val, err := prepareConfig(n)
		if err != nil {
			return nil, err
		}
		args = append(args, val)
	} else if len(n.use.Config) > 0 {
		return nil, assembleErr(n.field, n.use.Use, n.use.ID, StageDecode, fmt.Errorf("plugin does not take config"))
	}

	if spec.DepsType != nil {
		val, err := injectDeps(spec.DepsType, n.use.Deps, constructed)
		if err != nil {
			return nil, assembleErr(n.field, n.use.Use, n.use.ID, StageDeps, err)
		}
		args = append(args, val)
	} else if len(n.use.Deps) > 0 {
		return nil, assembleErr(n.field, n.use.Use, n.use.ID, StageDeps, fmt.Errorf("plugin does not take deps"))
	}

	val, err := callConstructor(fn, args...)
	if err != nil {
		return nil, assembleErr(n.field, n.use.Use, n.use.ID, StageConstruct, err)
	}
	return val, nil
}

func assignTarget(tv reflect.Value, p *plan, constructed map[string]any) error {
	for _, bind := range p.bindings {
		tf := bind.target
		fv := tv.Field(tf.index)
		if len(bind.uses) == 0 {
			if tf.slice && fv.IsNil() {
				fv.Set(reflect.MakeSlice(fv.Type(), 0, 0))
			}
			continue
		}
		if tf.slice {
			slice := reflect.MakeSlice(fv.Type(), 0, len(bind.uses))
			for _, use := range bind.uses {
				inst := constructed[use.ID]
				iv := reflect.ValueOf(inst)
				if err := checkImplements(tf.elemType, iv, use); err != nil {
					return assembleErr(tf.jsonName, use.Use, use.ID, StageTypeCheck, err)
				}
				slice = reflect.Append(slice, iv)
			}
			fv.Set(slice)
			continue
		}
		use := bind.uses[0]
		inst := constructed[use.ID]
		iv := reflect.ValueOf(inst)
		if err := checkImplements(tf.elemType, iv, use); err != nil {
			return assembleErr(tf.jsonName, use.Use, use.ID, StageTypeCheck, err)
		}
		if err := assignValue(fv, iv, use.ID); err != nil {
			return assembleErr(tf.jsonName, use.Use, use.ID, StageTypeCheck, err)
		}
	}
	return nil
}

func checkImplements(want reflect.Type, have reflect.Value, use config.PluginUse) error {
	if !have.IsValid() {
		return fmt.Errorf("plugin %q result is nil and cannot satisfy %s", use.Use, want)
	}
	if want.Kind() == reflect.Interface {
		if !have.Type().Implements(want) {
			return fmt.Errorf("plugin %q result %s does not implement %s", use.Use, have.Type(), want)
		}
		return nil
	}
	if !have.Type().AssignableTo(want) {
		return fmt.Errorf("plugin %q result %s is not assignable to %s", use.Use, have.Type(), want)
	}
	return nil
}

func staticTypeCheck(want, ret reflect.Type, use config.PluginUse) error {
	if ret.Kind() == reflect.Interface {
		return nil
	}
	if want.Kind() == reflect.Interface {
		if !ret.Implements(want) {
			return fmt.Errorf("plugin %q return type %s does not implement %s", use.Use, ret, want)
		}
		return nil
	}
	if !ret.AssignableTo(want) {
		return fmt.Errorf("plugin %q return type %s is not assignable to %s", use.Use, ret, want)
	}
	return nil
}

func checkRuntimeType(want reflect.Type, value any, use config.PluginUse) error {
	return checkImplements(want, reflect.ValueOf(value), use)
}
