package evo_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bannedCallsProbe reaches every banned-call shape the scan must see: a
// dot-import (whose calls carry no selector), an aliased import, and the
// context deadline constructors that take a cause.
const bannedCallsProbe = `package probe

import (
	"context"
	. "path/filepath"
	t "time"
)

func probe() {
	_ = Glob
	_ = t.Now
	_, _ = context.WithTimeoutCause(context.Background(), 1, nil)
	_, _ = context.WithDeadlineCause(context.Background(), t.Time{}, nil)
}
`

// TestTouchesSeesEveryBannedCallShape proves the scan flags a dot-import of a
// watched package outright, still follows an aliased import, and bans the
// context deadline constructors that take a cause.
func TestTouchesSeesEveryBannedCallShape(t *testing.T) {
	file := filepath.Join(t.TempDir(), "probe.go")
	if err := os.WriteFile(file, []byte(bannedCallsProbe), 0o600); err != nil {
		t.Fatalf("write probe: %v", err)
	}
	found, err := touches(file, neverBanned, bannedCalls)
	if err != nil {
		t.Fatalf("touches: %v", err)
	}
	report := strings.Join(found, "\n")
	for _, want := range []string{
		"dot-import path/filepath",
		"call time.Now",
		"call context.WithTimeoutCause",
		"call context.WithDeadlineCause",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("scan missed %q; found:\n%s", want, report)
		}
	}
}
