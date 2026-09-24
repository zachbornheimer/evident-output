package engine

// siblings is one parent's registry of the names its children claimed —
// the root (per declaration scope) or one Group/Sequence. §3.1 makes a
// repeated name under one parent a duplicate sibling, never a
// get-or-create, and this is the one place that rule is decided at every
// level. Tasks and containers are separate namespaces: a Task and a Group
// may share a name, but a Group and a Sequence may not.
type siblings struct {
	tasks      map[string]struct{}
	containers map[string]struct{}
}

func (s *siblings) taskTaken(name string) bool {
	_, ok := s.tasks[name]
	return ok
}

func (s *siblings) claimTask(name string) {
	if s.tasks == nil {
		s.tasks = make(map[string]struct{})
	}
	s.tasks[name] = struct{}{}
}

func (s *siblings) containerTaken(name string) bool {
	_, ok := s.containers[name]
	return ok
}

func (s *siblings) claimContainer(name string) {
	if s.containers == nil {
		s.containers = make(map[string]struct{})
	}
	s.containers[name] = struct{}{}
}

// siblingsLocked returns the registry children of parent claim names in;
// parent nil is the root of declaration scope. Callers must already hold
// o.mu.
func (o *Output) siblingsLocked(parent *tasksState, scope string) *siblings {
	if parent != nil {
		return &parent.names
	}
	if o.rootNames == nil {
		o.rootNames = make(map[string]*siblings)
	}
	s := o.rootNames[scope]
	if s == nil {
		s = &siblings{}
		o.rootNames[scope] = s
	}
	return s
}
