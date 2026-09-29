package evo

import "github.com/zachbornheimer/evident-output/internal/core"

// Event is an immutable journal record.
//
// Aliased into internal/core alongside the rest of the data model — see
// Snapshot's doc comment (snapshot.go) for why.
type Event = core.Event
