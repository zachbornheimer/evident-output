package evo_test

import (
	"reflect"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestNilTaskHandleIsSafeForEveryMethod: a nil *TaskHandle is documented
// safe for the type as a whole, so every exported method must be a no-op
// on it, not a panic. Reflection walks the whole method set, so a verb
// added later is covered without editing this test.
func TestNilTaskHandleIsSafeForEveryMethod(t *testing.T) {
	var nilHandle *evo.TaskHandle
	for _, handle := range []*evo.TaskHandle{nilHandle, {}} {
		v := reflect.ValueOf(handle)
		for i := range v.NumMethod() {
			m := v.Type().Method(i)
			t.Run(m.Name, func(t *testing.T) {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("(%#v).%s panicked: %v", handle, m.Name, r)
					}
				}()
				fn := v.Method(i)
				args := make([]reflect.Value, 0, fn.Type().NumIn())
				for j := range fn.Type().NumIn() {
					if fn.Type().IsVariadic() && j == fn.Type().NumIn()-1 {
						break
					}
					args = append(args, reflect.Zero(fn.Type().In(j)))
				}
				fn.Call(args)
			})
		}
	}
}
