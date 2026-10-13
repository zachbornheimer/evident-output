package evo

import (
	"fmt"
	"io"

	"github.com/zachbornheimer/evident-output/internal/project/wire"
)

func init() {
	// Single source of truth: PublishedRelease, pinned once into the
	// v2 wire encoder path (spec §32.1/§35's evo_version).
	SetWireEvoVersion(PublishedRelease)
}

// WriteJSON serializes result as the stable v2 "evo.run" wire document plus
// one trailing newline (spec §53) — the HTTP/embedding counterpart of
// FormatJSON's automatic Stdout write. It never serializes internal
// snapshots directly, and applies the same redaction Result's Conclusion
// already carries. HTTP status (or any other transport-level outcome) is
// the embedding application's own concern; Evo's outcome/exit semantics
// stay in the body (Conclusion.State/ExitCode).
func WriteJSON(w io.Writer, result Result) error {
	body, err := wire.EncodeRun(result, PublishedRelease)
	if err != nil {
		return fmt.Errorf("evo: encode evo.run document: %w", err)
	}
	body = append(body, '\n')
	_, err = w.Write(body)
	return err
}

// PublishedRelease is the single source of truth for the current published
// module and MCP pin used in install guidance.
//
// When cutting a release:
//  1. Promote CHANGELOG ## Unreleased → ## [X.Y.Z] (Keep a Changelog).
//  2. Set PublishedRelease to the new tag (e.g. "v0.2.11").
//  3. Prefer: mise run test && mise run cut-release
//  4. Tag that commit; do not move prior tags.
//
// version_drift_test.go enforces the portable surface stays synchronized.
const PublishedRelease = "v1.0.0"
