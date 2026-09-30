package apisurface

import "testing"

func TestIdent_WalkLines(t *testing.T) {
	cases := []struct {
		line, want string
	}{
		{"type Option", "Option"},
		{"type Config.Stdout", "Config.Stdout"},
		{"value EventSchemaVersion", "EventSchemaVersion"},
		{"func AlsoWrite(w io.Writer)  Option", "AlsoWrite"},
		{"func (Output) Cancel(reason string)", "Output.Cancel"},
		{"func (Failure) Error()  string", "Failure.Error"},
		{"func (TaskHandle) After(preds ...any)  *TaskHandle", "TaskHandle.After"},
		{"not a walk line", ""},
	}
	for _, c := range cases {
		if got := Ident(c.line); got != c.want {
			t.Errorf("Ident(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}
