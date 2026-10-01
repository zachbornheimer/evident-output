package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/engine"
	"github.com/zachbornheimer/evident-output/internal/fingerprint"
)

// basisInput is the sealed union TaskHandle.Basis accepts: File, Tree, and
// Fingerprint (Value, App).
type basisInput interface {
	isBasisInput()
	basisSource() engine.BasisSource
}

func (File) isBasisInput()        {}
func (Tree) isBasisInput()        {}
func (Fingerprint) isBasisInput() {}

func (f File) basisSource() engine.BasisSource        { return engine.FileBasis(f.Path) }
func (t Tree) basisSource() engine.BasisSource        { return engine.TreeBasis(t.Path) }
func (f Fingerprint) basisSource() engine.BasisSource { return engine.FingerprintBasis(f.inner) }

// Basis declares this Task's freshness inputs. With a shared StateDir, a
// Task whose Basis identities are unchanged since its last success is
// current and its Define callback does not run. Basis is content identity
// only: it neither orders Tasks (that is After) nor locks anything. Call it
// before Define; a call after Define records ErrBasisAfterDefine and is
// ignored.
func (t *TaskHandle) Basis(inputs ...basisInput) *TaskHandle {
	sources := make([]engine.BasisSource, 0, len(inputs))
	for _, in := range inputs {
		if in != nil {
			sources = append(sources, in.basisSource())
		}
	}
	t.impl().Basis(sources...)
	return t
}

// ErrBasisAfterDefine is recorded when Basis is called after Define.
var ErrBasisAfterDefine = engine.ErrBasisAfterDefine

// Fingerprint is a non-filesystem Basis identity, built by Value or App.
type Fingerprint struct{ inner fingerprint.Fingerprint }

// Fingerprint observes the identity's current value.
func (f Fingerprint) Fingerprint(ctx context.Context) (FingerprintValue, error) {
	return f.inner.Fingerprint(ctx)
}

// FingerprintValue is one Fingerprint's observed identity: a stable
// machine Kind, a stable non-secret Key, and a SHA-256 Digest.
type FingerprintValue = fingerprint.FingerprintValue

// Value fingerprints a caller-supplied scalar (string, bool, any signed/
// unsigned integer, any finite float, time.Time, or []byte) under a stable,
// safe name. Only the digest and name are ever persisted, never the raw
// value. Any other type is an error when the Fingerprint is taken.
func Value(name string, v any) Fingerprint { return Fingerprint{inner: fingerprint.Value(name, v)} }

// App fingerprints the running application itself: the executable's bytes
// when readable, else its Go build ID, else an error when the Fingerprint
// is taken. Include it in a Task's Basis only when the application's own
// implementation is a semantic input to that Task's result.
func App() Fingerprint { return Fingerprint{inner: fingerprint.App()} }
