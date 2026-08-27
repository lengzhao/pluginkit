package build

import (
	"errors"
	"reflect"
)

// ErrNoContributionsCollector is returned when result contains contributors
// but WireSetter or WireContributions could not wire any non-nil collector.
var ErrNoContributionsCollector = errors.New("pluginkit wire: contributors found but no collector wired")

// ContributionsSetter receives contributions of type T collected from result.
// Host frameworks can adopt this naming convention and wire with WireSetter.
type ContributionsSetter[T any] interface {
	SetContributions([]T) error
}

// WireContributions collects every instance assignable to Contributor from
// result and calls attach once per non-nil instance assignable to Collector.
// When there are no contributors, attach is not called. When contributors exist
// but no collector is wired, ErrNoContributionsCollector is returned.
// nil result or attach is a no-op.
func WireContributions[Contributor, Collector any](
	result *Result,
	attach func(Collector, []Contributor) error,
) error {
	if result == nil || attach == nil {
		return nil
	}
	contributions := Collect[Contributor](result)
	if len(contributions) == 0 {
		return nil
	}
	wired := false
	for _, collector := range Collect[Collector](result) {
		if isNil(collector) {
			continue
		}
		wired = true
		if err := attach(collector, contributions); err != nil {
			return err
		}
	}
	if !wired {
		return ErrNoContributionsCollector
	}
	return nil
}

// WireSetter collects contributions of type T and calls SetContributions on
// every non-nil instance that implements ContributionsSetter[T]. When there are
// no contributors, SetContributions is not called. When contributors exist
// but no collector is wired, ErrNoContributionsCollector is returned. nil
// result is a no-op.
func WireSetter[T any](result *Result) error {
	if result == nil {
		return nil
	}
	contributions := Collect[T](result)
	if len(contributions) == 0 {
		return nil
	}
	wired := false
	for _, inst := range result.Instances {
		setter, ok := inst.Value.(ContributionsSetter[T])
		if !ok || isNil(setter) {
			continue
		}
		wired = true
		if err := setter.SetContributions(contributions); err != nil {
			return err
		}
	}
	if !wired {
		return ErrNoContributionsCollector
	}
	return nil
}

func isNil[T any](v T) bool {
	val := reflect.ValueOf(v)
	if !val.IsValid() {
		return true
	}
	switch val.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface, reflect.Chan, reflect.Func:
		return val.IsNil()
	default:
		return false
	}
}
