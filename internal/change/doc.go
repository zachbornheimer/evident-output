// Package change decides what is done: it reconciles files, runs commands,
// applies patches and performs opaque effects, publishes each result
// atomically, and holds the claims that keep concurrent changes apart.
//
// It imports only the standard library, internal/record, internal/graph,
// internal/freshness and the facades. What it cannot reach it asks of the
// run around it through the interfaces it declares (Host).
package change
