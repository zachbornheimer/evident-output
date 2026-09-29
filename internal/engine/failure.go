package engine

// Failure is a recorded negative outcome's error value. Error() is the
// summary plus evidence; Unwrap() reaches a wrapped cause so errors.Is/As
// keep working. Failf and Blockf were removed in 1.1: Fail and Block are
// statements, and a Define callback returns a plain error. Attach a remedy
// with TaskHandle.Next / TaskHandle.NextCommand.
type Failure struct {
	err   error
	cause error
	// facade holds this Failure's public wrapper (see FacadeSlot).
	facade FacadeSlot
}

// Error returns the rendered failure message.
func (f *Failure) Error() string {
	if f == nil || f.err == nil {
		return ""
	}
	return f.err.Error()
}

// Unwrap reaches the wrapped cause, so errors.Is/As traverse through a
// Failure exactly as they do through fmt.Errorf.
func (f *Failure) Unwrap() error {
	if f == nil {
		return nil
	}
	return f.cause
}
