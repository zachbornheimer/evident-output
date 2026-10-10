// This file owns Basis observation: turning Fingerprints into the canonical
// BasisRecords a manifest compares, under the caller's read claims.

package freshness

import (
	"context"
	"encoding/hex"
	"fmt"
	"sort"
)

// Claimer holds the claims an observation needs. The caller implements it:
// freshness does not own claims, so it asks for them through this interface
// and never imports the package that does.
type Claimer interface {
	// HoldRead runs observe while a read claim on path is held, and releases
	// the claim however observe ends.
	HoldRead(ctx context.Context, path string, observe func(context.Context) error) error
}

// ObserveBasis fingerprints basis for one tracked operation (spec §11.1) and
// returns the records canonicalized. Each filesystem entry is observed while
// holding a read claim on its path (ZYS-840), so an observation never
// interleaves with a conflicting File commit: it sees the whole state before
// the commit or the whole state after it. Claims are taken one entry at a
// time, never nested.
func ObserveBasis(ctx context.Context, claims Claimer, basis []Fingerprint) ([]BasisRecord, error) {
	observed := make([]Fingerprint, len(basis))
	for i, b := range basis {
		observed[i] = b
		if path, isPath := PathOf(b); isPath {
			observed[i] = claimedObservation{claims: claims, path: path, inner: b}
		}
	}
	return basisRecordsFrom(ctx, observed)
}

// claimedObservation is a filesystem Fingerprint observed under a read claim
// on its own path.
type claimedObservation struct {
	claims Claimer
	path   string
	inner  Fingerprint
}

func (c claimedObservation) Fingerprint(ctx context.Context) (FingerprintValue, error) {
	var value FingerprintValue
	err := c.claims.HoldRead(ctx, c.path, func(context.Context) error {
		observedValue, observeErr := c.inner.Fingerprint(ctx)
		value = observedValue
		return observeErr
	})
	return value, err
}

// basisRecordsFrom fingerprints every entry in basis (spec §11.1) and
// returns them canonicalized by (Kind, Key) — Basis order is semantically
// irrelevant (§11.1). A duplicate (Kind, Key) pair is a programmer error
// (§11.1).
func basisRecordsFrom(ctx context.Context, basis []Fingerprint) ([]BasisRecord, error) {
	records := make([]BasisRecord, 0, len(basis))
	for _, b := range basis {
		v, err := b.Fingerprint(ctx)
		if err != nil {
			return nil, fmt.Errorf("evo: Basis: %w", err)
		}
		records = append(records, BasisRecord{
			Kind:   string(v.Kind),
			Key:    v.Key,
			Digest: hex.EncodeToString(v.Digest[:]),
		})
	}
	return canonicalBasis(records)
}

// canonicalBasis puts records in canonical (kind, key) order and refuses a
// repeated (kind, key): a programmer error (§11.1).
func canonicalBasis(records []BasisRecord) ([]BasisRecord, error) {
	SortBasisRecords(records)
	for i := 1; i < len(records); i++ {
		if records[i].Kind == records[i-1].Kind && records[i].Key == records[i-1].Key {
			return nil, fmt.Errorf("evo: Basis: duplicate (kind=%s, key=%s)", records[i].Kind, records[i].Key)
		}
	}
	return records, nil
}

// SortBasisRecords puts records in canonical (kind, key) order.
func SortBasisRecords(records []BasisRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].Kind != records[j].Kind {
			return records[i].Kind < records[j].Kind
		}
		return records[i].Key < records[j].Key
	})
}

// BasisRecordsEqual compares two already-canonicalized Basis slices
// element-wise — canonicalization makes a straightforward positional
// comparison correct rather than needing its own set-equality pass.
func BasisRecordsEqual(a, b []BasisRecord) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// PathOutputDigest fingerprints path's current on-disk content (spec
// §8.3/§11.1) for storage as, or comparison against, a File operation's
// tracked output record.
func PathOutputDigest(ctx context.Context, path string) (string, error) {
	v, err := FSPath(path).Fingerprint(ctx)
	if err != nil {
		return "", fmt.Errorf("evo: output %q: %w", path, err)
	}
	return hex.EncodeToString(v.Digest[:]), nil
}
