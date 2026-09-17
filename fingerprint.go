package evo

import "github.com/zachbornheimer/evident-output/internal/fingerprint"

// Fingerprint observes one Basis input's current content identity
// (spec §11.1). Implementations must not mutate the world.
//
// Aliased into internal/fingerprint alongside FSPath/Value/App — the same
// pattern action.go/fact.go use for the rest of the data model.
type Fingerprint = fingerprint.Fingerprint

// FingerprintValue is one Fingerprint's observed identity: a stable
// machine Kind, a stable non-secret Key, and a SHA-256 Digest.
type FingerprintValue = fingerprint.FingerprintValue

// FSPath fingerprints the filesystem content at path: a regular file's
// type-marked byte digest, a directory's deterministic Merkle digest over
// sorted entries, a symlink's target text (never followed), or a stable
// "missing" digest — see internal/fingerprint.FSPath.
func FSPath(path string) Fingerprint { return fingerprint.FSPath(path) }

// Value fingerprints a caller-supplied scalar (string, bool, any signed/
// unsigned integer, any finite float, time.Time, or []byte) under a stable,
// safe name — see internal/fingerprint.Value. Only the digest and name are
// ever persisted, never the raw value.
func Value(name string, v any) Fingerprint { return fingerprint.Value(name, v) }

// App fingerprints the running application itself — see
// internal/fingerprint.App. Include it in an operation's Basis only when
// the application's own implementation is a semantic input to that
// operation's result.
func App() Fingerprint { return fingerprint.App() }
