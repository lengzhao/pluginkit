package manager

import (
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
