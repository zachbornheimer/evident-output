// Package fixture is the hermetic stand-in for the daemon layer.
package fixture

import (
	"context"
	"io"
)

// Serve runs the daemon, writing its log to log, until ctx is cancelled. It
// returns nil on a clean stop.
func Serve(ctx context.Context, log io.Writer) error {
	_, _ = io.WriteString(log, "listening\n")
	<-ctx.Done()
	return nil
}
