package manager

import (
	"context"
	"fmt"
	"reflect"

	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/pluginkit/build"
)

func validatePlan(doc Document) error {
	spec, ok := pluginkit.Lookup(doc.Plugin.Use)
	if !ok {
		return fmt.Errorf("unknown plugin kind %q", doc.Plugin.Use)
	}
	return build.ValidatePlan(doc.ToGraph(), doc.RootID, spec.ReturnType)
}

func validateDocument(ctx context.Context, doc Document, validateBuild func(context.Context, Document) error) error {
	if err := doc.Validate(); err != nil {
		return err
	}
	if err := validatePlan(doc); err != nil {
		return err
	}
	if validateBuild != nil {
		return validateBuild(ctx, doc)
	}
	return nil
}

func validateTree(node PluginNode) error {
	desc, ok := pluginkit.Describe(node.Use)
	if !ok {
		return fmt.Errorf("unknown plugin kind %q", node.Use)
	}
	extByName := map[string]pluginkit.FieldDescription{}
	for _, ext := range desc.Extensions {
		extByName[ext.Name] = ext
	}
	for name, raw := range node.Deps {
		ext, ok := extByName[name]
		if !ok {
			return fmt.Errorf("plugin %q has unknown dep %q", node.Use, name)
		}
		if ext.List {
			items, err := decodeDepList(raw)
			if err != nil {
				return fmt.Errorf("plugin %q deps.%s: %w", node.Use, name, err)
			}
			for i, item := range items {
				if err := checkKindCompatible(ext.Type, item.Use); err != nil {
					return fmt.Errorf("plugin %q deps.%s[%d]: %w", node.Use, name, i, err)
				}
				if err := validateTree(item); err != nil {
					return err
				}
			}
			continue
		}
		child, err := decodeDepNode(raw)
		if err != nil {
			return fmt.Errorf("plugin %q deps.%s: %w", node.Use, name, err)
		}
		if err := checkKindCompatible(ext.Type, child.Use); err != nil {
			return fmt.Errorf("plugin %q deps.%s: %w", node.Use, name, err)
		}
		if err := validateTree(child); err != nil {
			return err
		}
	}
	for _, ext := range desc.Extensions {
		if ext.Optional {
			continue
		}
		if _, ok := node.Deps[ext.Name]; !ok {
			return fmt.Errorf("plugin %q missing required dep %q", node.Use, ext.Name)
		}
	}
	return nil
}

func checkKindCompatible(want reflect.Type, kind string) error {
	spec, ok := pluginkit.Lookup(kind)
	if !ok {
		return fmt.Errorf("unknown plugin kind %q", kind)
	}
	if !compatibleReturnType(want, spec.ReturnType) {
		return fmt.Errorf("plugin %q return type %s does not satisfy %s", kind, spec.ReturnType, want)
	}
	return nil
}

func compatibleReturnType(want, ret reflect.Type) bool {
	if ret == nil {
		return false
	}
	if ret.Kind() == reflect.Interface {
		return true
	}
	if want.Kind() == reflect.Interface {
		return ret.Implements(want)
	}
	return ret.AssignableTo(want)
}
