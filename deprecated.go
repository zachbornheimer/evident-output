package evo

// ItemHandle is Task's shipped v0.2.x name for a fact-check entity (a task
// resolved without ever running). It is a zero-cost alias — TaskHandle is
// the one entity/handle type — kept only so v0.2.x call sites still compile.
// Where old code chained OK().Because(text), spell the same outcome as
// Done(text).
//
// Deprecated: Use TaskHandle. Will be removed in v1.0.
type ItemHandle = TaskHandle

// Item declares a fact-check entity: a Task that is typically resolved
// directly (Done/Warn/Block/Fail/Skip) without ever calling Phase/Progress.
// name is a printf format when args are present (fmt.Sprintf semantics).
// Where old code chained OK().Because(text), spell the same outcome as
// Done(text).
//
// Deprecated: Use Task. Will be removed in v1.0.
func (o *Output) Item(name string, args ...any) *TaskHandle {
	return o.Task(name, args...)
}

// Item declares a fact-check entity under this scope's naming.
//
// Deprecated: Use Scope.Task. Will be removed in v1.0.
func (s *Scope) Item(name string, args ...any) *TaskHandle {
	if s == nil || s.out == nil {
		return &TaskHandle{}
	}
	return s.out.Scope(s.name).Task(name, args...)
}

// Item declares a fact-check entity on the default instance.
//
// Deprecated: Use Task. Will be removed in v1.0.
func Item(name string, opts ...EntityOption) *TaskHandle {
	args := make([]any, len(opts))
	for i, opt := range opts {
		args[i] = opt
	}
	return Default().Item(name, args...)
}

// CaptureOption is EvidenceOption's shipped v0.2.16 name (C9: finishes the
// Capture->Evidence rename in the option surface).
//
// Deprecated: Use EvidenceOption. Will be removed in v1.0.
type CaptureOption = EvidenceOption

// CaptureStream is EvidenceStream's shipped v0.2.16 name.
//
// Deprecated: Use EvidenceStream. Will be removed in v1.0.
type CaptureStream = EvidenceStream

// CaptureStreamCombined, CaptureStreamStdout, CaptureStreamStderr are
// EvidenceStream's shipped v0.2.16 constant names.
//
// Deprecated: Use EvidenceStreamCombined/Stdout/Stderr. Will be removed in v1.0.
const (
	CaptureStreamCombined = EvidenceStreamCombined
	CaptureStreamStdout   = EvidenceStreamStdout
	CaptureStreamStderr   = EvidenceStreamStderr
)

// MaxCaptureBytes is MaxEvidenceBytes's shipped v0.2.16 name.
//
// Deprecated: Use MaxEvidenceBytes. Will be removed in v1.0.
func MaxCaptureBytes(n int) EvidenceOption {
	return MaxEvidenceBytes(n)
}
