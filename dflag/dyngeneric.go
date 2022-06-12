// Copyright 2022 Fortio Authors

package dflag

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/exp/constraints"
)

// DynamicFlagValue interface is a tag to know if a type is dynamic or not.
type DynamicFlagValue interface {
	IsDynamicFlag() bool
}

// DynamicJSONFlagValue is a tag interface for JSON dynamic flags.
type DynamicJSONFlagValue interface {
	IsJSON() bool
}

// DynamicFlagValueTag is a struct all dynamic flag inherit for marking they are dynamic.
type DynamicFlagValueTag struct{}

// IsDynamicFlag always returns true.
func (*DynamicFlagValueTag) IsDynamicFlag() bool {
	return true
}

// A flag is dynamic if it implements DynamicFlagValue (which all the dyn* do)

// IsFlagDynamic returns whether the given Flag has been created in a Dynamic mode.
func IsFlagDynamic(f *flag.Flag) bool {
	df, ok := f.Value.(DynamicFlagValue)
	if !ok {
		return false
	}
	return df.IsDynamicFlag() // will clearly return true if it exists
}

// ---- Generics section ---

type DynValueTypes interface {
	bool | time.Duration | float64 | int64 | string
}

type DynValue[T DynValueTypes] struct {
	DynamicFlagValueTag
	av           atomic.Value
	ready        bool
	syncNotifier bool
	validator    func(T) error
	notifier     func(oldValue T, newValue T)
	mutator      func(inp T) T
	inpMutator   func(inp string) string
}

func Dyn[T DynValueTypes](flagSet *flag.FlagSet, name string, value T, usage string) *DynValue[T] {
	dynValue := &DynValue[T]{}
	dynValue.av.Store(value)
	dynValue.inpMutator = strings.TrimSpace // default so parsing of numbers etc works well
	dynValue.ready = true
	flagSet.Var(dynValue, name, usage)
	flagSet.Lookup(name).DefValue = fmt.Sprintf("%v", value)
	return dynValue
}

// IsBoolFlag lets the flag parsing know that -flagname is enough to turn to true.
func (d *DynValue[T]) IsBoolFlag() bool {
	var v T
	switch any(v).(type) {
	case bool:
		return true
	default:
		return false
	}
}

// Get retrieves the value in a thread-safe manner.
func (d *DynValue[T]) Get() T {
	var zero T
	if !d.ready {
		// avoid crashing when String()->Get() is called by flagset.PrintDefaults
		// which happens in error case (and is tested in nildptr_test.go)
		return zero
	}
	return d.av.Load().(T)
}

// Parse converts from string to our supported types (it's the missing generics strconv.Parse[T]).
func Parse[T any](input string) (val T, err error) {
	switch any(val).(type) {
	case bool:
		var v bool
		v, err = strconv.ParseBool(input)
		val = any(v).(T)
	case int64:
		var v int64
		v, err = strconv.ParseInt(strings.TrimSpace(input), 0, 64)
		val = any(v).(T)
	case float64:
		var v float64
		v, err = strconv.ParseFloat(strings.TrimSpace(input), 64)
		val = any(v).(T)
	case time.Duration:
		var v time.Duration
		v, err = time.ParseDuration(input)
		val = any(v).(T)
	case string:
		val = any(input).(T)
	default:
		err = fmt.Errorf("unexpected type %T", val)
	}
	return
}

// Set updates the value from a string representation in a thread-safe manner.
// This operation may return an error if the provided `input` doesn't parse, or the resulting value doesn't pass an
// optional validator.
// If a notifier is set on the value, it will be invoked in a separate go-routine.
func (d *DynValue[T]) Set(rawInput string) error {
	input := rawInput
	if d.inpMutator != nil {
		input = d.inpMutator(rawInput)
	}
	val, err := Parse[T](input)
	if err != nil {
		return err
	}
	if d.mutator != nil {
		val = d.mutator(val)
	}
	if d.validator != nil {
		if err := d.validator(val); err != nil {
			return err
		}
	}
	oldVal := d.av.Swap(val).(T)
	if d.notifier != nil {
		if d.syncNotifier {
			d.notifier(oldVal, val)
		} else {
			go d.notifier(oldVal, val)
		}
	}
	return nil
}

// WithValidator adds a function that checks values before they're set.
// Any error returned by the validator will lead to the value being rejected.
// Validators are executed on the same go-routine as the call to `Set`.
func (d *DynValue[T]) WithValidator(validator func(T) error) *DynValue[T] {
	d.validator = validator
	return d
}

// WithNotifier adds a function is called every time a new value is successfully set.
// Each notifier is executed in a new go-routine.
func (d *DynValue[T]) WithNotifier(notifier func(oldValue T, newValue T)) *DynValue[T] {
	d.notifier = notifier
	return d
}

// WithSyncNotifier adds a function is called synchronously every time a new value is successfully set.
func (d *DynValue[T]) WithSyncNotifier(notifier func(oldValue T, newValue T)) *DynValue[T] {
	d.notifier = notifier
	d.syncNotifier = true
	return d
}

// Type is an indicator of what this flag represents.
func (d *DynValue[T]) Type() string {
	var v T
	return fmt.Sprintf("dyn_%T", v)
}

// String returns the canonical string representation of the type.
func (d *DynValue[T]) String() string {
	return fmt.Sprintf("%v", d.Get())
}

// WithValueMutator adds a function that changes the value of a flag as needed.
func (d *DynValue[T]) WithValueMutator(mutator func(inp T) T) *DynValue[T] {
	d.mutator = mutator
	return d
}

// WithInputMutator changes the default input string processing (TrimSpace).
func (d *DynValue[T]) WithInputMutator(mutator func(inp string) string) *DynValue[T] {
	d.inpMutator = mutator
	return d
}

// ValidateDynFloat64Range returns a validator that checks if the float value is in range.
func ValidateRange[T constraints.Ordered](fromInclusive T, toInclusive T) func(T) error {
	return func(value T) error {
		if value > toInclusive || value < fromInclusive {
			return fmt.Errorf("value %v not in [%v, %v] range", value, fromInclusive, toInclusive)
		}
		return nil
	}
}
