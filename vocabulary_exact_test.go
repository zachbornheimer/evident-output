package evo_test

import (
	"reflect"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestVocabulary_TaskHandleHasNoRetiredMethods is the closed-set check for
// the 1.x contract table "Not part of evo". EntityState Done stays: that
// word is a snapshot state, not the removed Task.Done method.
func TestVocabulary_TaskHandleHasNoRetiredMethods(t *testing.T) {
	handle := reflect.TypeFor[*evo.TaskHandle]()
	output := reflect.TypeFor[*evo.Output]()
	retiredOnTask := []string{
		"Done", "Blockf", "Failf", "Warn", "Step", "Kept",
		"Record", "RecordLabel", "RecordName",
		"Add", "Create", "Delete", "Push", "Remove", "Update", "Write",
		"Lock", "Unlock", "ReadLock", "WriteLock", "IsLocked",
	}
	for _, name := range retiredOnTask {
		if _, ok := handle.MethodByName(name); ok {
			t.Errorf("TaskHandle.%s is exported; the contract removes it", name)
		}
	}
	for _, name := range []string{"Failf", "Warn"} {
		if _, ok := output.MethodByName(name); ok {
			t.Errorf("Output.%s is exported; the contract removes it", name)
		}
	}
}

// TestVocabulary_CanonicalMethodsExist checks the words the contract says
// are the whole of the caller-facing task and file API.
func TestVocabulary_CanonicalMethodsExist(t *testing.T) {
	handle := reflect.TypeFor[*evo.TaskHandle]()
	for _, name := range []string{
		"After", "Define", "Wait", "Block", "Fail", "Skipped",
		"Summary", "Fact", "Problem", "Doing", "Progress", "Verify",
	} {
		if _, ok := handle.MethodByName(name); !ok {
			t.Errorf("TaskHandle.%s is missing", name)
		}
	}
	basis, ok := reflect.TypeFor[evo.FileSpec]().FieldByName("Basis")
	if !ok || !basis.IsExported() {
		t.Fatal("FileSpec.Basis is missing; Basis is a canonical word")
	}
	for _, name := range []string{"File", "Files", "Patch", "Exec", "Effect", "Init", "Main"} {
		if reflect.ValueOf(map[string]any{
			"File": evo.File, "Files": evo.Files, "Patch": evo.Patch,
			"Exec": evo.Exec, "Effect": evo.Effect, "Init": evo.Init, "Main": evo.Main,
		}[name]).IsNil() {
			t.Errorf("%s is nil", name)
		}
	}
}
