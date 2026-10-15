// Package freshness decides whether work is needed: it fingerprints the
// inputs an operation depends on, remembers what the last successful Run
// saw in a manifest, and judges each operation satisfied or stale.
//
// It imports only the standard library, internal/record, internal/graph and
// the facades. What it cannot reach it asks of its caller through interfaces
// it declares (Claimer, TaskView).
package freshness
