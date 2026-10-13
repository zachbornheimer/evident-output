// Package wire owns the v2 machine wire contract (spec §32.1–§38): the
// "evo.run" final document and the "evo.event" JSONL line. Every type here
// is an explicit wire contract, never an alias onto an internal runtime
// struct (§33: "No public HTTP/JSON encoder marshals internal Go structs
// directly") — a runtime field rename must not silently change the wire
// shape, and a wire field rename must not silently change runtime code.
//
// This package also holds the frozen 0.4/0.3 encoders (EncodeJSON,
// EncodeJSONL) that used to live beside it in internal/render.
package wire
