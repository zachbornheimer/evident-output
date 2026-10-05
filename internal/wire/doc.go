// Package wire owns the v2 machine wire contract (spec §32.1–§38): the
// "evo.run" final document and the "evo.event" JSONL line. Every type here
// is an explicit wire contract, never an alias onto an internal runtime
// struct (§33: "No public HTTP/JSON encoder marshals internal Go structs
// directly") — a runtime field rename must not silently change the wire
// shape, and a wire field rename must not silently change runtime code.
//
// This is schema v2, additive alongside the existing "0.4"/"0.3" encoders in
// internal/render (schema v1 final JSON, v1 JSONL) — see internal/render's
// package doc for why that pair stays byte-identical. Nothing in this
// package is reachable from internal/render's call graph, and nothing in
// internal/render is reachable from here.
package wire
