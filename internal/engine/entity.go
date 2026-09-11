package engine

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
//	out.Task("download base image")
func iD(id string) EntityOption {
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

// scope is a namespaced declaration handle for plugins and subsystems.
//
// Contract (honest limits):
//
//   - Qualifies evo.ID keys as "scope.key" for stable machine identity.
//
//   - Exposes only Task and Tasks — operations that actually take the namespace.
//
//   - Is NOT a security sandbox: plugins holding *Output bypass scope entirely.
//
//   - Session Capture, Writer, and SlogHandler stay on *Output (shared session).
//
//     registry := out.scope("registry")
//     registry.Task("credentials").Done()
//     // key → "registry.auth"
type scope struct {
	out  *Output
	name string
}

// scope returns a namespaced handle. It does not render a visible section.
func (o *Output) scope(name string) *scope {
	if o == nil {
		return &scope{name: name}
	}
	return &scope{out: o, name: name}
}

// Name returns the scope path segment.
func (s *scope) Name() string {
	if s == nil {
		return ""
	}
	return s.name
}

// Task declares a task; optional evo.ID is prefixed with the scope name.
// name is a printf format when args are present (fmt.Sprintf semantics).
func (s *scope) Task(name string) *TaskHandle {
	if s == nil || s.out == nil {
		return &TaskHandle{}
	}
	return s.out.taskScoped(name, s.name)
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
