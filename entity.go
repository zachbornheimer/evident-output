package evo

import "fmt"

// EntityOption configures Task declaration (stable keys).
// The common path remains Task("label"); options are platform-scale.
type EntityOption interface {
	applyEntity(*entityOpts)
}

type entityOpts struct {
	key   string
	phase string
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

// StartPhase declares a task with its first Phase already set, in one call —
// declare-then-phase collapsed into the declaration itself:
//
//	out.Task("download base image", evo.StartPhase("resolving tag"))
//
// is exactly out.Task("download base image").Phase("resolving tag"), with no
// separate statement (and no gap where the task sits Pending) between them.
func StartPhase(text string) EntityOption {
	return entityOptionFunc(func(o *entityOpts) { o.phase = text })
}

// formatEntityName splits a Task/Item declaration's trailing args into the
// fmt.Sprintf arguments and the EntityOption values mixed in among them (e.g.
// evo.ID("x")), then resolves the printf-style name — no args means name is
// returned verbatim, so a plain evo.Task("branches") never runs through
// Sprintf and can't misfire on a literal "%" in a caller's label.
func formatEntityName(name string, args []any) (string, []EntityOption) {
	if len(args) == 0 {
		return name, nil
	}
	var opts []EntityOption
	var fmtArgs []any
	for _, a := range args {
		if opt, ok := a.(EntityOption); ok {
			opts = append(opts, opt)
			continue
		}
		fmtArgs = append(fmtArgs, a)
	}
	if len(fmtArgs) == 0 {
		return name, opts
	}
	return fmt.Sprintf(name, fmtArgs...), opts
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
//
//   - Qualifies evo.ID keys as "scope.key" for stable machine identity.
//
//   - Exposes only Task and Tasks — operations that actually take the namespace.
//
//   - Is NOT a security sandbox: plugins holding *Output bypass Scope entirely.
//
//   - Session Capture, Writer, and SlogHandler stay on *Output (shared session).
//
//     registry := out.Scope("registry")
//     registry.Task("credentials", evo.ID("auth")).Done()
//     // key → "registry.auth"
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

// Task declares a task; optional evo.ID is prefixed with the scope name.
// name is a printf format when args are present (fmt.Sprintf semantics).
func (s *Scope) Task(name string, args ...any) *TaskHandle {
	if s == nil || s.out == nil {
		return &TaskHandle{}
	}
	formatted, opts := formatEntityName(name, args)
	return s.out.taskScoped(formatted, s.name, opts...)
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
