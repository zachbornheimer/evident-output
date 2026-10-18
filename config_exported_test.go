package evo_test

import (
	"reflect"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// TestConfigHasOnlyExportedFields pins CONTRACT.md §1: Config is the run's
// settings as a caller sees them. Every field is exported, so nothing a caller
// cannot set shapes a run; internal settings live in the private config that
// Config resolves into once.
func TestConfigHasOnlyExportedFields(t *testing.T) {
	typ := reflect.TypeFor[evo.Config]()
	for i := range typ.NumField() {
		if f := typ.Field(i); !f.IsExported() {
			t.Errorf("Config.%s is unexported; Config holds only what a caller sets", f.Name)
		}
	}
}
