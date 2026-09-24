package core

// CancelCause is a cancelled run's stable, machine-readable cause — the
// "evo.run" document's cancellation.cause (DEC-CANCEL-007). The human
// form of the same cause is Conclusion.Explanation.
type CancelCause string

const (
	// CancelCauseUser: SIGINT/SIGTERM reached the run. It is the only
	// cause 1.2 emits; a caller-owned lifecycle and its causes are
	// deferred behind ZYS-947.
	CancelCauseUser CancelCause = "user"
)

// CancelCauseOf reports why c's run was cancelled; empty when the run was
// not cancelled or nothing recorded a cause.
func CancelCauseOf(c Conclusion) CancelCause { return c.cancelCause }

// SetCancelCause records cause on a cancelled c. Any other outcome keeps
// no cause, so the wire never reports a cause for a run that completed.
func SetCancelCause(c *Conclusion, cause CancelCause) {
	if c.State == StateCancelled {
		c.cancelCause = cause
	}
}
