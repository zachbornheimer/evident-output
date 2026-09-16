// Package fingerprint computes observation-only content identities for
// evo.File/evo.Exec Basis and operation-definition hashing (spec §11.1).
//
// Every constructor here (FSPath, Value, App) is read-only: none of them
// mutate the world, and none of them accept or leak secret material — only
// a stable Kind/Key label and a SHA-256 Digest are ever persisted.
package fingerprint

import (
	"context"
	"crypto/sha256"
)

// Kind is a Fingerprint's stable machine category, persisted and safe to
// appear in Debug/JSON output.
type Kind string

const (
	// KindFSPath identifies a filesystem-path Fingerprint (FSPath).
	KindFSPath Kind = "fs_path"
	// KindValue identifies a caller-supplied scalar Fingerprint (Value).
	KindValue Kind = "value"
	// KindApp identifies the running-application Fingerprint (App).
	KindApp Kind = "app"
)

// Digest is a Fingerprint's SHA-256 content identity.
type Digest = [32]byte

// FingerprintValue is one Fingerprint's observed identity: what kind of
// thing it is (Kind), which one (Key), and its content digest.
type FingerprintValue struct {
	Kind   Kind
	Key    string
	Digest Digest
}

// Fingerprint observes one Basis input's current content identity.
// Implementations must not mutate the world (spec §11.1).
type Fingerprint interface {
	Fingerprint(ctx context.Context) (FingerprintValue, error)
}

// sum256 is the one seam every constructor in this package hashes through,
// so every digest in the package is demonstrably a plain SHA-256 sum of its
// documented preimage.
func sum256(preimage []byte) Digest {
	return sha256.Sum256(preimage)
}
