package core

import (
	"reflect"
	"testing"
)

// taskFieldRole classifies a TaskSnapshot field for IsZeroInformationTask.
type taskFieldRole int

const (
	// roleInformation: a non-zero value is something a reader must see, so
	// it makes the Task carry information.
	roleInformation taskFieldRole = iota
	// roleIdentity: names, orders, or timestamps the row; never information
	// by itself.
	roleIdentity
	// roleOutcome: the state, resolution, and synthetic marker the
	// predicate checks explicitly.
	roleOutcome
)

// zeroInformationFields is every TaskSnapshot field with its role. A field
// missing here fails TestEveryTaskSnapshotFieldIsClassified: a new field
// the predicate silently ignored would hide rows that carry it.
var zeroInformationFields = map[string]taskFieldRole{
	"ID":              roleIdentity,
	"Key":             roleIdentity,
	"Name":            roleIdentity,
	"State":           roleOutcome,
	"Phase":           roleInformation,
	"ActivityAt":      roleIdentity,
	"liveFirstSeenAt": roleIdentity,
	"Progress":        roleInformation,
	"Summary":         roleInformation,
	"Problems":        roleInformation,
	"Warnings":        roleInformation,
	"Facts":           roleInformation,
	"Verification":    roleInformation,
	"Actions":         roleInformation,
	"Skipped":         roleInformation,
	"Collection":      roleIdentity,
	"Declaration":     roleIdentity,
	"Resolution":      roleOutcome,
	// Evidence is the proof behind an AlreadySatisfied resolution, which a
	// proven no-op always carries; the row states nothing new with it.
	"Evidence":  roleIdentity,
	"synthetic": roleOutcome,
}

func TestEveryTaskSnapshotFieldIsClassified(t *testing.T) {
	typ := reflect.TypeFor[TaskSnapshot]()
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		if _, ok := zeroInformationFields[name]; !ok {
			t.Errorf("TaskSnapshot.%s has no zero-information role: add it to zeroInformationFields and, if it is information, to IsZeroInformationTask", name)
		}
	}
}

// TestEveryInformationFieldDefeatsZeroInformation sets each information
// field on an otherwise zero-information Task and requires the predicate to
// notice.
func TestEveryInformationFieldDefeatsZeroInformation(t *testing.T) {
	base := TaskSnapshot{State: Done, Resolution: ResolutionNoWork}
	if !IsZeroInformationTask(base) {
		t.Fatal("a bare NoWork Done Task must be zero-information")
	}
	for name, role := range zeroInformationFields {
		if role != roleInformation {
			continue
		}
		task := base
		setNonZero(t, reflect.ValueOf(&task).Elem().FieldByName(name))
		if IsZeroInformationTask(task) {
			t.Errorf("TaskSnapshot.%s is information, but a Task carrying it still reads as zero-information", name)
		}
	}
}

func setNonZero(t *testing.T, v reflect.Value) {
	t.Helper()
	switch v.Kind() {
	case reflect.String:
		v.SetString("x")
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
	case reflect.Struct:
		for i := range v.NumField() {
			if f := v.Field(i); f.CanSet() && f.CanInt() {
				f.SetInt(1)
			}
		}
	default:
		t.Fatalf("setNonZero: unhandled kind %s", v.Kind())
	}
}
