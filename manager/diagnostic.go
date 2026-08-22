package manager

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/pluginkit/build"
)

// Diagnostic 是钉到稳定 path 上的分层校验结果。
type Diagnostic struct {
	Path     string `json:"path"`
	Severity string `json:"severity"`
	Stage    string `json:"stage"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

func collectDiagnostics(doc Document) []Diagnostic {
	diags := structureDiagnostics(doc)
	diags = append(diags, planDiagnostics(doc)...)
	return diags
}

func structureDiagnostics(doc Document) []Diagnostic {
	var diags []Diagnostic
	if doc.RootID != "" {
		if _, ok := doc.Shared[doc.RootID]; ok {
			diags = append(diags, structureDiag("root", "id_conflict",
				fmt.Sprintf("shared instance id %q conflicts with rootId", doc.RootID)))
		}
	}
	resolve := doc.instanceResolver()
	diags = append(diags, nodeStructureDiags("root", doc.Plugin, resolve)...)
	for _, id := range sortedKeys(doc.Shared) {
		diags = append(diags, nodeStructureDiags("shared."+id, doc.Shared[id], resolve)...)
	}
	return diags
}

func planDiagnostics(doc Document) []Diagnostic {
	err := validatePlan(doc)
	if err == nil {
		return nil
	}
	var be *build.Error
	if !errors.As(err, &be) {
		return []Diagnostic{planDiag("root", "plan", err.Error())}
	}
	path, mapped := planErrorPath(doc, be.ID)
	if !mapped {
		path = "root"
	}
	return []Diagnostic{planDiag(path, "plan", err.Error())}
}

func planErrorPath(doc Document, id string) (string, bool) {
	if id == "" {
		return "root", false
	}
	if id == doc.RootID {
		return "root", true
	}
	if _, ok := doc.Shared[id]; ok {
		return "shared." + id, true
	}
	if doc.RootID != "" && strings.HasPrefix(id, doc.RootID+".") {
		translated, ok := translateBuildSuffix(strings.TrimPrefix(id, doc.RootID+"."))
		if !ok {
			return "root", false
		}
		return "root." + translated, true
	}
	best := ""
	for sharedID := range doc.Shared {
		if sharedID == "" || !strings.HasPrefix(id, sharedID+".") {
			continue
		}
		if len(sharedID) > len(best) {
			best = sharedID
		}
	}
	if best != "" {
		translated, ok := translateBuildSuffix(strings.TrimPrefix(id, best+"."))
		if !ok {
			return "root", false
		}
		return "shared." + best + "." + translated, true
	}
	return "root", false
}

// translateBuildSuffix 把 build 内联后缀 a[i].b 转成 deps.a[i].deps.b。
func translateBuildSuffix(suffix string) (string, bool) {
	if suffix == "" {
		return "", false
	}
	var parts []string
	rest := suffix
	for rest != "" {
		name, index, leftover, err := parseNameIndex(rest)
		if err != nil || name == "" {
			return "", false
		}
		if index >= 0 {
			parts = append(parts, fmt.Sprintf("%s[%d]", name, index))
		} else {
			parts = append(parts, name)
		}
		if leftover == "" {
			break
		}
		if !strings.HasPrefix(leftover, ".") {
			return "", false
		}
		rest = leftover[1:]
		if rest == "" {
			return "", false
		}
	}
	return "deps." + strings.Join(parts, ".deps."), true
}

func sortedDepNames(deps map[string]any) []string {
	names := make([]string, 0, len(deps))
	for name := range deps {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func nodeStructureDiags(path string, node PluginNode, resolve instanceResolver) []Diagnostic {
	desc, ok := pluginkit.Describe(node.Use)
	if !ok {
		return []Diagnostic{structureDiag(path, "unknown_kind",
			fmt.Sprintf("unknown plugin kind %q", node.Use))}
	}

	extByName := make(map[string]pluginkit.FieldDescription, len(desc.Extensions))
	for _, ext := range desc.Extensions {
		extByName[ext.Name] = ext
	}

	var diags []Diagnostic
	for _, name := range sortedDepNames(node.Deps) {
		raw := node.Deps[name]
		slotPath := path + ".deps." + name
		ext, ok := extByName[name]
		if !ok {
			diags = append(diags, structureDiag(slotPath, "unknown_dep",
				fmt.Sprintf("plugin %q has unknown dep %q", node.Use, name)))
			continue
		}
		if ext.List {
			diags = append(diags, listDepDiags(slotPath, node.Use, name, ext, raw, resolve)...)
			continue
		}
		diags = append(diags, singleDepDiags(slotPath, node.Use, name, ext, raw, resolve)...)
	}
	for _, ext := range desc.Extensions {
		if ext.Optional {
			continue
		}
		if _, ok := node.Deps[ext.Name]; !ok {
			diags = append(diags, structureDiag(path+".deps."+ext.Name, "missing_dep",
				fmt.Sprintf("plugin %q missing required dep %q", node.Use, ext.Name)))
		}
	}
	return diags
}

func listDepDiags(slotPath, pluginUse, name string, ext pluginkit.FieldDescription, raw any, resolve instanceResolver) []Diagnostic {
	items, err := decodeDepList(raw)
	if err != nil {
		return []Diagnostic{structureDiag(slotPath, "invalid_dep",
			fmt.Sprintf("plugin %q deps.%s: %s", pluginUse, name, err))}
	}
	var diags []Diagnostic
	for i, item := range items {
		itemPath := fmt.Sprintf("%s[%d]", slotPath, i)
		if refID, ok := item.(string); ok {
			diags = append(diags, refDiags(itemPath, ext.Type, refID, resolve)...)
			continue
		}
		child, err := decodeDepNode(item)
		if err != nil {
			diags = append(diags, structureDiag(itemPath, "invalid_dep",
				fmt.Sprintf("plugin %q deps.%s[%d]: %s", pluginUse, name, i, err)))
			continue
		}
		diags = append(diags, filledNodeDiags(itemPath, ext.Type, child, resolve)...)
	}
	return diags
}

func singleDepDiags(slotPath, pluginUse, name string, ext pluginkit.FieldDescription, raw any, resolve instanceResolver) []Diagnostic {
	if refID, ok := raw.(string); ok {
		return refDiags(slotPath, ext.Type, refID, resolve)
	}
	child, err := decodeDepNode(raw)
	if err != nil {
		return []Diagnostic{structureDiag(slotPath, "invalid_dep",
			fmt.Sprintf("plugin %q deps.%s: %s", pluginUse, name, err))}
	}
	return filledNodeDiags(slotPath, ext.Type, child, resolve)
}

func filledNodeDiags(path string, want reflect.Type, child PluginNode, resolve instanceResolver) []Diagnostic {
	diags := kindDiags(path, want, child.Use)
	for _, d := range diags {
		if d.Code == "unknown_kind" {
			return diags
		}
	}
	return append(diags, nodeStructureDiags(path, child, resolve)...)
}

func refDiags(path string, want reflect.Type, refID string, resolve instanceResolver) []Diagnostic {
	target, ok := resolve(refID)
	if !ok {
		return []Diagnostic{structureDiag(path, "unknown_ref",
			fmt.Sprintf("unknown instance reference %q", refID))}
	}
	return kindDiags(path, want, target.Use)
}

func kindDiags(path string, want reflect.Type, kind string) []Diagnostic {
	spec, ok := pluginkit.Lookup(kind)
	if !ok {
		return []Diagnostic{structureDiag(path, "unknown_kind",
			fmt.Sprintf("unknown plugin kind %q", kind))}
	}
	if !compatibleReturnType(want, spec.ReturnType) {
		return []Diagnostic{structureDiag(path, "incompatible",
			fmt.Sprintf("plugin %q return type %s does not satisfy %s", kind, spec.ReturnType, want))}
	}
	return nil
}

func structureDiag(path, code, message string) Diagnostic {
	return Diagnostic{
		Path:     path,
		Severity: "error",
		Stage:    "structure",
		Code:     code,
		Message:  message,
	}
}

func planDiag(path, code, message string) Diagnostic {
	return Diagnostic{
		Path:     path,
		Severity: "error",
		Stage:    "plan",
		Code:     code,
		Message:  message,
	}
}
