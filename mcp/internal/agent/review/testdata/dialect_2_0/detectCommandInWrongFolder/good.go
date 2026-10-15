package app

import (
	"github.com/spf13/cobra"
	"github.com/zachbornheimer/zq/internal/purge"
)

// purgeCommand is a one-return delegate; internal/purge owns the RunE body.
func purgeCommand() *cobra.Command {
	return purge.Command()
}
