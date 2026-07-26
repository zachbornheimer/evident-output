package evo

// Snapshots returns a buffered channel of immutable snapshots.
// The channel is closed when the output is closed or finished.
// Callers should not block the library; buffer absorbs bursts.
func (o *Output) Snapshots() <-chan Snapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.snapCh == nil {
		o.snapCh = make(chan Snapshot, 16)
	}
	// Publish current state immediately.
	select {
	case o.snapCh <- o.snapshotLocked():
	default:
	}
	return o.snapCh
}

func (o *Output) publishSnapshotLocked() {
	if o.snapCh == nil {
		return
	}
	snap := o.snapshotLocked()
	select {
	case o.snapCh <- snap:
	default:
		// Drop intermediate snapshots under pressure; latest will follow.
	}
}

func (o *Output) closeSnapshotsLocked() {
	if o.snapCh == nil {
		return
	}
	// Final snapshot
	select {
	case o.snapCh <- o.snapshotLocked():
	default:
	}
	close(o.snapCh)
	o.snapCh = nil
}
