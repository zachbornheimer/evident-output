package engine

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/fingerprint"
	"github.com/zachbornheimer/evident-output/internal/manifest"
)

// observeBasis fingerprints basis for one tracked operation (spec §11.1).
// Each filesystem entry is observed while holding a read claim on its path
// (ZYS-840), so an observation never interleaves with a conflicting File
// commit: it sees the whole state before the commit or the whole state
// after it. Claims are taken one entry at a time, never nested.
func (o *Output) observeBasis(ctx context.Context, basis []fingerprint.Fingerprint) ([]manifest.BasisRecord, error) {
	observed := make([]fingerprint.Fingerprint, len(basis))
	for i, b := range basis {
		observed[i] = b
		if path, isPath := fingerprint.PathOf(b); isPath {
			observed[i] = claimedObservation{out: o, path: path, inner: b}
		}
	}
	return basisRecordsFrom(ctx, observed)
}

// claimedObservation is a filesystem Fingerprint observed under a read
// claim on its own path.
type claimedObservation struct {
	out   *Output
	path  string
	inner fingerprint.Fingerprint
}

func (c claimedObservation) Fingerprint(ctx context.Context) (fingerprint.FingerprintValue, error) {
	var value fingerprint.FingerprintValue
	err := c.out.holdResource(ctx, FSResource(c.path), resourceRead, func(context.Context) error {
		observedValue, observeErr := c.inner.Fingerprint(ctx)
		value = observedValue
		return observeErr
	})
	return value, err
}
