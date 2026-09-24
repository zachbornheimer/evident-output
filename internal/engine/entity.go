package engine

// scope is a namespaced declaration handle for plugins and subsystems.
//
// Contract (honest limits):
//
//   - Qualifies explicit Task keys as "scope.key" for stable machine
//     identity.
//
//   - Exposes only Task and Tasks — operations that actually take the namespace.
//
//   - Is NOT a security sandbox: plugins holding *Output bypass scope entirely.
//
//   - Session Capture, Writer, and SlogHandler stay on *Output (shared session).
//
//     registry := out.scope("registry")
//     registry.Task("credentials").Define(checkCredentials)
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

// Task declares a task named name in this scope.
func (s *scope) Task(name string) *TaskHandle {
	if s == nil || s.out == nil {
		return &TaskHandle{}
	}
	return s.out.taskScoped(name, s.name, "")
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
