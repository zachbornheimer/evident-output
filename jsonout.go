package evo

import (
	"fmt"
	"io"

	"github.com/zachbornheimer/evident-output/internal/engine"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

func init() {
	// Single source of truth: PublishedRelease (release.go), pinned once
	// into the engine's v2 wire encoder path (spec §32.1/§35's evo_version).
	engine.SetWireEvoVersion(PublishedRelease)
}

// ParseFormat parses "human", "data", "external", "json", or "jsonl"
// (case-insensitive, surrounding whitespace ignored) into a Format — the
// entry point a host CLI's own --format/--json flag binds to (spec §32.1).
// Evo does not parse os.Args itself, and never infers FormatJSON merely
// because Stdout is a pipe.
func ParseFormat(s string) (Format, error) { return engine.ParseFormat(s) }

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
