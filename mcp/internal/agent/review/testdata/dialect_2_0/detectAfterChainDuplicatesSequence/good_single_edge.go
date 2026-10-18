// One After between unrelated Tasks is an exceptional edge, not a chain.
package dag002

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func setupInfra(network, database *evo.Task) {
	database.After(network)
	database.Define(migrateDatabase)
}

func migrateDatabase(ctx context.Context) error { return nil }
