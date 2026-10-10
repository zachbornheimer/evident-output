package graph

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

func TestDeclaredInCallbackIsFalseOutsideAnyCallback(t *testing.T) {
	g := New(record.NewRun())

	if g.DeclaredInCallback() {
		t.Error("DeclaredInCallback = true for a caller outside every callback")
	}
}

func TestDeclaredInCallbackIsTrueInsideATaskCallback(t *testing.T) {
	g := New(record.NewRun())
	task := declare(g, "callback")
	var inside bool
	submitWork(g, task, settlingWork(g, task, func() error { inside = g.DeclaredInCallback(); return nil }))

	g.Drain()

	if !inside {
		t.Error("DeclaredInCallback = false inside a Task callback")
	}
}

func TestDeclaredInCallbackIsFalseInsideAContainerBuilder(t *testing.T) {
	g := New(record.NewRun())
	group := g.AddContainer(nil, "group", false)
	inside := true
	gate := g.AddGate(group, func() error { inside = g.DeclaredInCallback(); return nil })
	g.Submit(gate, Work{})

	g.Drain()

	if inside {
		t.Error("DeclaredInCallback = true inside a container builder")
	}
}
