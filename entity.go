package evo

// EntityOption configures Item or Task declaration (stable keys).
// The common path remains Item("label") / Task("label"); options are platform-scale.
type EntityOption interface {
	applyEntity(*entityOpts)
}

type entityOpts struct {
	key string
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
//
//   - Qualifies evo.ID keys as "scope.key" for stable machine identity.
//
//   - Exposes only Item, Task, and Tasks — operations that actually take the namespace.
//
//   - Is NOT a security sandbox: plugins holding *Output bypass Scope entirely.
//
//   - Session Capture, Writer, and SlogHandler stay on *Output (shared session).
//
//     registry := out.Scope("registry")
//     registry.Item("credentials", evo.ID("auth")).OK()
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

// Tasks declares a task collection under this scope's naming (human name only;
// child tasks still take evo.ID with scope qualification via Scope.Task).
func (s *Scope) Tasks(name string) *Tasks {
	if s == nil || s.out == nil {
		return &Tasks{}
	}
	return s.out.Tasks(name)
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
