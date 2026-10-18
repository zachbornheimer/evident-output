// Package wire owns the v2 machine wire contract (schema/run.v3.json and schema/event.v2.json): the
// "evo.run" final document and the "evo.event" JSONL line. Every type here
// is an explicit wire contract, never an alias onto an internal runtime
// struct (§33: "No public HTTP/JSON encoder marshals internal Go structs
// directly") — a runtime field rename must not silently change the wire
// shape, and a wire field rename must not silently change runtime code.
package wire
