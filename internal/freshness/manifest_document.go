// This file owns the manifest document: the persisted reconciliation truth
// across Runs — which operation last ran, under which definition, with which
// Basis and output digests (spec §11.3-11.4). The manifest is cache-like:
// losing it must only cause safe re-execution, never a false claim that
// stale state is current.

package freshness

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ManifestSchemaVersion is the on-disk manifest-v1.json schema this package
// reads and writes. A document whose schema_version does not match is treated as
// a cache miss (never Evidence=true), not a fatal error.
const ManifestSchemaVersion = 1

// maxDocumentBytes bounds how large a manifest file this package will
// trust before refusing to parse it (spec §11.3's "size limits validated
// before use") — a corrupt or hostile file cannot force an unbounded read.
const maxDocumentBytes = 64 << 20 // 64 MiB

// ErrManifestCorrupt is returned by Load (and wrapped into a warning, never
// surfaced as Evidence) when the manifest file exists but cannot be
// trusted: invalid JSON, wrong schema version, or over maxDocumentBytes.
var ErrManifestCorrupt = errors.New("manifest: corrupt or unrecognized manifest file")

// BasisRecord is one committed Basis input's identity at commit time.
type BasisRecord struct {
	Kind   string `json:"kind"`
	Key    string `json:"key"`
	Digest string `json:"digest"` // hex-encoded SHA-256
}

// OutputRecord is one committed tracked-output resource's identity at
// commit time.
type OutputRecord struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Digest string `json:"digest"` // hex-encoded SHA-256
}

// OperationRecord is one committed operation's definition and the state it
// last verified.
type OperationRecord struct {
	Kind                  string         `json:"kind"` // e.g. "file"
	DefinitionFingerprint string         `json:"definition_fingerprint"`
	Basis                 []BasisRecord  `json:"basis,omitempty"`
	Outputs               []OutputRecord `json:"outputs,omitempty"`
}

// TaskRecord is one Task's committed reconciliation truth.
type TaskRecord struct {
	Key        string            `json:"key"`
	Operations []OperationRecord `json:"operations,omitempty"`
	// DefinitionFingerprint is this Task's own definition identity, set
	// only when the Task recorded no precise File/Exec/Patch Operations of
	// its own (ZYS-817 Decisions 2026-09-23: "opaque Define/task definition
	// identity automatically incorporates the application fingerprint as
	// conservative fallback with zero caller code"). A Task with at least
	// one Operation leaves this empty — precise per-operation provenance
	// already beats the fallback and must not be shadowed by it. It is
	// provenance only: it never lets a Run skip the Task's Define, because
	// application identity cannot prove opaque state (a push, an API call)
	// current.
	DefinitionFingerprint string `json:"definition_fingerprint,omitempty"`
}

// ApplicationRecord identifies the application and its fingerprint at
// commit time (spec §11.2).
type ApplicationRecord struct {
	ID          string `json:"id"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// ManifestDocument is the manifest file's full on-disk shape.
type ManifestDocument struct {
	SchemaVersion int                   `json:"schema_version"`
	Application   ApplicationRecord     `json:"application"`
	Tasks         map[string]TaskRecord `json:"tasks"`
}

// newDocument returns an empty, current-schema document for app.
func newDocument(app ApplicationRecord) ManifestDocument {
	return ManifestDocument{SchemaVersion: ManifestSchemaVersion, Application: app, Tasks: map[string]TaskRecord{}}
}

// decodeDocument parses and validates raw manifest bytes. Any failure
// (invalid JSON, schema mismatch, oversized input) is ErrManifestCorrupt — a safe
// cache miss, never a reason to treat stale content as current.
func decodeDocument(raw []byte) (ManifestDocument, error) {
	if len(raw) > maxDocumentBytes {
		return ManifestDocument{}, fmt.Errorf("%w: %d bytes exceeds %d byte limit", ErrManifestCorrupt, len(raw), maxDocumentBytes)
	}
	var doc ManifestDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ManifestDocument{}, fmt.Errorf("%w: %v", ErrManifestCorrupt, err)
	}
	if doc.SchemaVersion != ManifestSchemaVersion {
		return ManifestDocument{}, fmt.Errorf("%w: schema_version %d, want %d", ErrManifestCorrupt, doc.SchemaVersion, ManifestSchemaVersion)
	}
	if doc.Tasks == nil {
		doc.Tasks = map[string]TaskRecord{}
	}
	return doc, nil
}

// Warnf is how OpenManifest/Load report a safe cache miss (corrupt/unknown
// manifest) to the caller as a diagnostic instead of silently proceeding —
// callers that care surface it as a Fact/log line; callers that don't may
// ignore it. It is never an error: a miss is always safe to continue past.
type ManifestWarning struct {
	// Err is the underlying reason for the miss (e.g. ErrManifestCorrupt).
	Err error
}

func (w *ManifestWarning) Error() string { return fmt.Sprintf("manifest: %v", w.Err) }
func (w *ManifestWarning) Unwrap() error { return w.Err }
