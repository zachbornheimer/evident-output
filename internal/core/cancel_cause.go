package core

// CancelCause is a cancelled run's stable, machine-readable cause — the
// "evo.run" document's cancellation.cause (DEC-CANCEL-007). The human
// form of the same cause is Conclusion.Explanation.
type CancelCause string

const (
	// CancelCauseUser: SIGINT/SIGTERM reached a CLI run.
	CancelCauseUser CancelCause = "user"
	// CancelCauseCaller: an embedded run's caller cancelled its context
	// (an HTTP client disconnected, or the host shut the request down).
	CancelCauseCaller CancelCause = "caller"
	// CancelCauseDeadline: an embedded run's caller deadline passed.
	CancelCauseDeadline CancelCause = "deadline"
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
