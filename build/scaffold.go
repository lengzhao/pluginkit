package build

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/lengzhao/pluginkit"
)

// ScaffoldOptions 控制从 target struct 生成配置时的插件筛选。
// Whitelist 非空时只保留名单内的 kind；Blacklist 始终排除对应 kind。
type ScaffoldOptions struct {
	Whitelist []string
	Blacklist []string
}

// Scaffold 根据 target struct 的扩展点字段，自动生成 BuildInto 可接受的 plugins 配置。
// slice 字段会包含所有兼容插件；单值字段默认选第一个兼容插件。
// 插件实例 id 默认使用 kind，冲突时追加 -2、-3。
func Scaffold(target any, opts ScaffoldOptions) (map[string]any, error) {
	cfg, _, err := scaffoldConfig(target, opts)
	return cfg, err
}

type scaffoldContext struct {
	opts        ScaffoldOptions
	topFields   []targetField
	topLevelIDs map[string]string
	usedIDs     map[string]bool
	comments    map[string]string
}

func scaffoldConfig(target any, opts ScaffoldOptions) (map[string]any, map[string]string, error) {
	_, fields, err := inspectTarget(target)
	if err != nil {
		return nil, nil, err
	}
	ctx := &scaffoldContext{
		opts:        opts,
		topFields:   fields,
		topLevelIDs: make(map[string]string),
		usedIDs:     make(map[string]bool),
		comments:    make(map[string]string),
	}
	if err := ctx.prepareTopLevel(); err != nil {
		return nil, nil, err
	}

	cfg := make(map[string]any, len(fields))
	for _, tf := range fields {
		raw, err := ctx.scaffoldTopLevelField(tf)
		if err != nil {
			return nil, nil, err
		}
		if raw == nil {
			continue
		}
		cfg[tf.jsonName] = raw
	}
	return cfg, ctx.comments, nil
}

func (ctx *scaffoldContext) prepareTopLevel() error {
	for _, tf := range ctx.topFields {
		kinds := ctx.candidatesFor(tf.elemType)
		if len(kinds) == 0 {
			if tf.optional && !tf.slice {
				continue
			}
			if tf.slice {
				continue
			}
			return fmt.Errorf("no compatible plugin for field %q", tf.jsonName)
		}
		if tf.slice {
			continue
		}
		ctx.topLevelIDs[tf.jsonName] = ctx.allocateID(kinds[0])
	}
	return nil
}

func (ctx *scaffoldContext) scaffoldTopLevelField(tf targetField) (any, error) {
	kinds := ctx.candidatesFor(tf.elemType)
	if len(kinds) == 0 {
		if tf.optional && !tf.slice {
			return nil, nil
		}
		if tf.slice {
			return []any{}, nil
		}
		return nil, fmt.Errorf("no compatible plugin for field %q", tf.jsonName)
	}
	if tf.slice {
		items := make([]any, 0, len(kinds))
		for i, kind := range kinds {
			use, err := ctx.scaffoldPlugin(kind, fmt.Sprintf("%s[%d]", tf.jsonName, i))
			if err != nil {
				return nil, err
			}
			items = append(items, use)
		}
		return items, nil
	}

	kind := kinds[0]
	if len(kinds) > 1 {
		ctx.setComment(tf.jsonName+".use", alternativesComment(kinds))
	}
	id := ctx.topLevelIDs[tf.jsonName]
	if id == "" {
		id = ctx.allocateID(kind)
		ctx.topLevelIDs[tf.jsonName] = id
	}
	return ctx.scaffoldPluginWithID(kind, id, tf.jsonName)
}

func (ctx *scaffoldContext) scaffoldPlugin(kind, path string) (map[string]any, error) {
	return ctx.scaffoldPluginWithID(kind, ctx.allocateID(kind), path)
}

func (ctx *scaffoldContext) scaffoldPluginWithID(kind, id, path string) (map[string]any, error) {
	desc, ok := pluginkit.Describe(kind)
	if !ok {
		return nil, fmt.Errorf("unknown plugin kind %q", kind)
	}
	tmpl := desc.Template()
	use := map[string]any{
		"id":  id,
		"use": kind,
	}
	if cfg, ok := tmpl["config"]; ok && cfg != nil {
		use["config"] = cfg
	}
	deps, err := ctx.scaffoldDeps(desc.Extensions, path)
	if err != nil {
		return nil, err
	}
	if len(deps) > 0 {
		use["deps"] = deps
	}
	return use, nil
}

func (ctx *scaffoldContext) scaffoldDeps(extensions []pluginkit.FieldDescription, path string) (map[string]any, error) {
	if len(extensions) == 0 {
		return nil, nil
	}
	deps := make(map[string]any)
	for _, ext := range extensions {
		if ext.Optional {
			continue
		}
		depPath := path + ".deps." + ext.Name
		val, err := ctx.scaffoldDep(ext, depPath)
		if err != nil {
			return nil, err
		}
		if val != nil {
			deps[ext.Name] = val
		}
	}
	if len(deps) == 0 {
		return nil, nil
	}
	return deps, nil
}

func (ctx *scaffoldContext) scaffoldDep(ext pluginkit.FieldDescription, path string) (any, error) {
	if !ext.List {
		if refID, altKinds, ok := ctx.findTopLevelRef(ext.Type); ok {
			if len(altKinds) > 1 {
				ctx.setComment(path, alternativesComment(altKinds))
			}
			return refID, nil
		}
	}

	kinds := ctx.candidatesFor(ext.Type)
	if len(kinds) == 0 {
		return nil, fmt.Errorf("no compatible plugin for dep %q", ext.Name)
	}
	if ext.List {
		items := make([]any, 0, len(kinds))
		for i, kind := range kinds {
			use, err := ctx.scaffoldPlugin(kind, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, err
			}
			items = append(items, use)
		}
		return items, nil
	}

	kind := kinds[0]
	if len(kinds) > 1 {
		ctx.setComment(path+".use", alternativesComment(kinds))
	}
	return ctx.scaffoldPlugin(kind, path)
}

func (ctx *scaffoldContext) findTopLevelRef(want reflect.Type) (id string, altKinds []string, ok bool) {
	var matches []targetField
	for _, tf := range ctx.topFields {
		if tf.slice {
			continue
		}
		if !typesCompatible(want, tf.elemType) {
			continue
		}
		if _, exists := ctx.topLevelIDs[tf.jsonName]; !exists {
			continue
		}
		matches = append(matches, tf)
	}
	if len(matches) != 1 {
		return "", nil, false
	}
	tf := matches[0]
	kinds := ctx.candidatesFor(tf.elemType)
	return ctx.topLevelIDs[tf.jsonName], kinds, true
}

func (ctx *scaffoldContext) candidatesFor(want reflect.Type) []string {
	return filterKinds(pluginkit.CompatibleKinds(want), ctx.opts)
}

func (ctx *scaffoldContext) allocateID(kind string) string {
	id := kind
	if !ctx.usedIDs[id] {
		ctx.usedIDs[id] = true
		return id
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", kind, i)
		if !ctx.usedIDs[candidate] {
			ctx.usedIDs[candidate] = true
			return candidate
		}
	}
}

func (ctx *scaffoldContext) setComment(path, comment string) {
	if comment == "" {
		return
	}
	ctx.comments[path] = comment
}

func filterKinds(kinds []string, opts ScaffoldOptions) []string {
	white := toKindSet(opts.Whitelist)
	black := toKindSet(opts.Blacklist)
	out := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		if len(opts.Whitelist) > 0 && !white[kind] {
			continue
		}
		if black[kind] {
			continue
		}
		out = append(out, kind)
	}
	return out
}

func toKindSet(kinds []string) map[string]bool {
	set := make(map[string]bool, len(kinds))
	for _, kind := range kinds {
		set[kind] = true
	}
	return set
}

func typesCompatible(want, have reflect.Type) bool {
	if want == nil || have == nil {
		return false
	}
	if have == want {
		return true
	}
	if want.Kind() == reflect.Interface && have.Implements(want) {
		return true
	}
	if have.Kind() == reflect.Interface && want.Implements(have) {
		return true
	}
	return have.AssignableTo(want) || want.AssignableTo(have)
}

func alternativesComment(kinds []string) string {
	if len(kinds) <= 1 {
		return ""
	}
	return fmt.Sprintf("default: %s; alternatives: %s", kinds[0], strings.Join(kinds[1:], ", "))
}
