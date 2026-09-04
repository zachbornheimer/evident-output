package evo

import "github.com/zachbornheimer/evident-output/internal/core"

// FactRecord is a discovered name/value annotation — information, not work
// (user-13-problems.md Problem 8). Named FactRecord, not Fact, because Fact
// is the evo.Fact/TaskHandle.Fact verb (EffectRecord sets the naming
// precedent: the verb keeps the plain name, its stored shape gets Record).
// See TaskHandle.Fact and Output.Fact.
//
// Aliased into internal/core alongside the rest of the data model — see
// Snapshot's doc comment (snapshot.go) for why.
type FactRecord = core.Fact
