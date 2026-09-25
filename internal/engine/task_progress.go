package engine

// Progress sets absolute completed/total count progress.
// Counts use int (collection lengths, indices). For byte quantities use Bytes.
// Prefer absolute Progress over Advance so retries cannot double-count.
func (t *TaskHandle) Progress(completed, total int) *TaskHandle {
	return t.setProgress(int64(completed), int64(total), Determinate)
}

// Bytes sets absolute byte progress (units and rate formatting).
func (t *TaskHandle) Bytes(completed, total int64) *TaskHandle {
	return t.setProgress(completed, total, BytesKind)
}

func (t *TaskHandle) setProgress(completed, total int64, kind ProgressKind) *TaskHandle {
	return t.annotate(func(st *taskState) { t.applyProgressLocked(st, completed, total, kind) })
}

// applyProgressLocked reports whether the update was applied — false means a
// guard (invalid values, regression, sealed-total mismatch) rejected it and
// recorded misuse instead, so a caller can skip a paired update that would
// otherwise describe a progress change that never happened.
func (t *TaskHandle) applyProgressLocked(st *taskState, completed, total int64, kind ProgressKind) bool {
	if completed < 0 || total < 0 {
		t.out.recordMisuse(ErrInvalidProgress)
		return false
	}
	if total == 0 && completed != 0 {
		t.out.recordMisuse(ErrInvalidProgress)
		return false
	}
	if completed > total && total > 0 {
		t.out.recordMisuse(ErrInvalidProgress)
		return false
	}
	// Regression and sealing guards apply only while re-reporting the same
	// measurement kind (Determinate or Bytes); switching kind (e.g. Progress
	// then Bytes) is a deliberate re-declaration and resets both freely.
	if st.state == Running && st.progress.Kind != Indeterminate && st.progress.Total > 0 && kind == st.progress.Kind {
		if completed < st.progress.Completed {
			t.out.recordMisuse(ErrProgressRegression)
			return false
		}
		// Sealed total: once a nonzero total is reported for this kind, it
		// cannot change. Retry-safety depends on the denominator staying put.
		if total != st.progress.Total {
			t.out.recordMisuse(ErrInvalidProgress)
			return false
		}
	}
	st.progress = Progress{Kind: kind, Completed: completed, Total: total}
	st.activityAt = t.out.cfg.clock.Now()
	if st.state == Pending {
		t.out.promoteRunningLocked(st)
	}
	t.out.bumpLocked()
	t.out.appendEventLocked(Event{Type: "task.progress_changed", EntityID: t.id})
	// Progress is high-frequency: coalesce unless first frame.
	t.out.signalLiveLocked(false)
	t.out.emitTaskRunningProgressiveLocked(st, triggerProgress)
	return true
}
