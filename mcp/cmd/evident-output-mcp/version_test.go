package main

import "testing"

// TestResolvedVersion_LdflagsStampWins proves --version trusts the
// ldflags-stamped Version over any build-info fallback (mise's build task
// and install.go's `go install -C <dir>` stamp main.Version this way).
func TestResolvedVersion_LdflagsStampWins(t *testing.T) {
	orig := Version
	defer func() { Version = orig }()

	Version = "v0.4.7"
	if got := resolvedVersion(); got != "v0.4.7" {
		t.Fatalf("resolvedVersion() = %q, want the stamped v0.4.7", got)
	}
}

// TestResolvedVersion_NeverBareDevString proves the unresolved sentinel
// itself is never mistaken for a resolved version — evo-dialect-axes-report.md
// axis 4/12: a fresh --version printing the bare string "dev" cannot tell a
// stale host from a fresh one.
func TestResolvedVersion_NeverBareDevString(t *testing.T) {
	if installedVersionUnset != "dev" {
		t.Fatalf("installedVersionUnset = %q, want dev (the sentinel --version must never trust)", installedVersionUnset)
	}
}
