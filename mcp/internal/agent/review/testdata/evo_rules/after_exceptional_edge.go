// Fixture: §62 baseline. A single exceptional .After(...) edge (spec §6's
// own database.After(network) example), not a chain — zero EVO-* findings.
package afteredge

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func setupInfra(setup *evo.SequenceHandle, network, database *evo.TaskHandle) {
	database.After(network)
	database.Define(migrateDatabase)
}

func migrateDatabase(ctx context.Context) error { return nil }
