package engine

import "crypto/rand"

// runIDPrefix marks a run identity on the wire ("run_id", spec §16/§35).
const runIDPrefix = "run_"

// newRunID is the facade every Output draws its run identity from. The
// identity is random, not a per-process counter: concurrent embedded runs
// (spec §53, one Output per HTTP request) and runs from separate processes
// must never share a run_id a machine consumer correlates on.
var newRunID = func() string { return runIDPrefix + rand.Text() }
