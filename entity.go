package evo

import (
	"io"
	"log/slog"
)

// EntityOption configures Item or Task declaration (stable keys).
// The common path remains Item("label") / Task("label"); options are platform-scale.
type EntityOption interface {
	applyEntity(*entityOpts)
}

type entityOpts struct {
	key   string
	scope string
}

type entityOptionFunc func(*entityOpts)

func (f entityOptionFunc) applyEntity(o *entityOpts) { f(o) }

// ID sets a stable machine key independent of the human label.
// Labels may be localized or reworded; IDs should not.
//
//	out.Task("download base image", evo.ID("build.base-image.download"))
func ID(id string) EntityOption {
	return entityOptionFunc(func(o *entityOpts) { o.key = id })
}

func applyEntityOptions(opts []EntityOption) entityOpts {
	var o entityOpts
	for _, opt := range opts {
		if opt != nil {
			opt.applyEntity(&o)
		}
	}
	return o
}

// Scope is a namespaced declaration handle for plugins and subsystems.
//
// Contract (honest limits):
//   - Qualifies evo.ID keys as "scope.key" for stable machine identity.
//   - Exposes Item, Task, Capture, Writer, and SlogHandler on the shared Output.
//   - Is NOT a security sandbox: plugins holding *Output bypass Scope entirely;
//     there is no separate stream, Fail isolation, or nested authority boundary.
//
//	registry := out.Scope("registry")
//	registry.Item("credentials", evo.ID("auth")).OK()
//	// key → "registry.auth"
type Scope struct {
	out  *Output
	name string
}

// Scope returns a namespaced handle. It does not render a visible section.
func (o *Output) Scope(name string) *Scope {
	if o == nil {
		return &Scope{name: name}
	}
	return &Scope{out: o, name: name}
}

// Name returns the scope path segment.
func (s *Scope) Name() string {
	if s == nil {
		return ""
	}
	return s.name
}

// Item declares an item; optional evo.ID is prefixed with the scope name.
func (s *Scope) Item(name string, opts ...EntityOption) *Item {
	if s == nil || s.out == nil {
		return &Item{}
	}
	return s.out.itemScoped(name, s.name, opts...)
}

// Task declares a task; optional evo.ID is prefixed with the scope name.
func (s *Scope) Task(name string, opts ...EntityOption) *Task {
	if s == nil || s.out == nil {
		return &Task{}
	}
	return s.out.taskScoped(name, s.name, opts...)
}

// Writer returns a human message writer (plugins must not open their own TTY).
func (s *Scope) Writer() io.Writer {
	if s == nil || s.out == nil {
		return io.Discard
	}
	return s.out.Writer()
}

// SlogHandler returns a slog bridge for this output session.
func (s *Scope) SlogHandler(min slog.Leveler) slog.Handler {
	if s == nil || s.out == nil {
		return slog.NewTextHandler(io.Discard, nil)
	}
	return s.out.SlogHandler(min)
}

// Capture returns a process-output sink on the shared session (not stream-isolated).
// Prefer Task.Capture when evidence belongs to one operation.
func (s *Scope) Capture(opts ...CaptureOption) *Capture {
	if s == nil || s.out == nil {
		return newCapture(nil, "", "", opts...)
	}
	return s.out.Capture(opts...)
}

func qualifyKey(scope, key string) string {
	if key == "" {
		return ""
	}
	if scope == "" {
		return key
	}
	prefix := scope + "."
	if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
		return key
	}
	return prefix + key
}
