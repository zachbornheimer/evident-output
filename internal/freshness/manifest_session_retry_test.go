package freshness

import (
	"errors"
	"sync/atomic"
	"testing"
)

// A Save whose write fails keeps the store
// so the session's Close retries the write (5fc26070 / C30-085).
func TestSessionCloseRetriesAWriteSaveCouldNotFinish(t *testing.T) {
	state := t.TempDir()
	session := sessionOn(state)
	store, err := session.Store(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	transient := errors.New("transient")
	var writes atomic.Int32
	store.write = func(raw []byte) error {
		if writes.Add(1) == 1 {
			return transient
		}
		return store.writeAtomic(raw)
	}
	store.StageTask(session.Application(), TaskRecord{Key: "k"})
	if err := session.Save(); !errors.Is(err, transient) {
		t.Fatalf("Save() = %v, want the transient write error", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil: Close must retry the write Save could not finish", err)
	}
	reopened, err := OpenManifest(t.Context(), ManifestConfig{StateDir: state}, fakeEnvironment{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	if _, ok := reopened.Task("k"); !ok {
		t.Fatal("record missing: the failed Save was never retried")
	}
}
