package evo

import (
	"github.com/zachbornheimer/evident-output/internal/freshness"
)

// Fingerprint observes one Basis input's current content identity
// (spec §11.1). Implementations must not mutate the world.
type Fingerprint = freshness.Fingerprint

// FingerprintValue is one Fingerprint's observed identity: a stable
// machine Kind, a stable non-secret Key, and a SHA-256 Digest.
type FingerprintValue = freshness.FingerprintValue

// FSPath fingerprints the filesystem content at path: a regular file's
// type-marked byte digest, a directory's deterministic Merkle digest over
// sorted entries, a symlink's target text (never followed), or a stable
// "missing" digest. Permission, mtime, and owner changes never change the
// digest; an unreadable path is an error, never reported as missing.
func FSPath(path string) Fingerprint { return freshness.FSPath(path) }

// Value fingerprints a caller-supplied scalar (string, bool, any signed/
// unsigned integer, any finite float, time.Time, or []byte) under a stable,
// safe name. Only the digest and name are ever persisted, never the raw
// value. Any other type is an error when the Fingerprint is taken.
func Value(name string, v any) Fingerprint { return freshness.Value(name, v) }

// App fingerprints the running application itself: the executable's bytes
// when readable, else its Go build ID, else an error when the Fingerprint
// is taken. Include it in a Task's Basis only when the application's own
// implementation is a semantic input to that Task's result.
func App() Fingerprint { return freshness.App() }
