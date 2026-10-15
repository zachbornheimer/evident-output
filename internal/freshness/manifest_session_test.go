package freshness

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSessionOpensTheManifestOnceAndCachesIt(t *testing.T) {
	state := t.TempDir()
	opens := 0
	session := NewManifestSession(func() ManifestConfig {
		opens++
		return ManifestConfig{AppID: "app", StateDir: state}
	})
	if session.Opened() != nil {
		t.Fatal("a new session has opened nothing")
	}
	first, err := session.Store(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.Store(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if first != second || session.Opened() != first {
		t.Fatal("Store must hand back the one opened store")
	}
	if session.Application().ID != "app" {
		t.Fatalf("application = %+v, want the configured id", session.Application())
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
}

// A Run that never asks for the manifest never creates it.
func TestSessionThatNeverOpenedLeavesNoManifestFile(t *testing.T) {
	state := t.TempDir()
	session := sessionOn(state)
	if err := session.Save(); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(state, manifestFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat manifest = %v, want it absent", err)
	}
}

// Save releases the Run's lock once the history is on disk, so another Run
// can open the same manifest without waiting for a Close, and this session
// reopens on its next use.
func TestSessionSaveReleasesTheLockAndALaterStoreReopens(t *testing.T) {
	state := t.TempDir()
	session := sessionOn(state)
	store, err := session.Store(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	store.StageTask(session.Application(), TaskRecord{Key: "k"})
	if err := session.Save(); err != nil {
		t.Fatal(err)
	}
	if session.Opened() != nil {
		t.Fatal("Save must release the store")
	}
	rival, err := OpenManifest(t.Context(), ManifestConfig{StateDir: state}, fakeEnvironment{})
	if err != nil {
		t.Fatalf("another Run could not open the saved manifest: %v", err)
	}
	if _, ok := rival.Task("k"); !ok {
		t.Fatal("Save left the staged record off disk")
	}
	if err := rival.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := session.Store(t.Context())
	if err != nil || reopened == nil {
		t.Fatalf("reopen = (%v, %v), want a store", reopened, err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSessionRefusesToOpenAfterClose(t *testing.T) {
	session := sessionOn(t.TempDir())
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := session.Store(t.Context())
	if store != nil || !errors.Is(err, ErrManifestSessionClosed) {
		t.Fatalf("Store after Close = (%v, %v), want (nil, ErrManifestSessionClosed)", store, err)
	}
}

func TestSessionStatesEachWarningOnce(t *testing.T) {
	session := sessionOn(t.TempDir())
	if !session.FirstMissWarning() || session.FirstMissWarning() {
		t.Fatal("the cache-miss warning must be first exactly once")
	}
	if !session.FirstUnsavedWarning() || session.FirstUnsavedWarning() {
		t.Fatal("the not-saved warning must be first exactly once")
	}
}
