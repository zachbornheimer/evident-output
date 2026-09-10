// Fixture for the facade-detector false positive: a Config.defaults method
// that assigns Logf to a fmt.Fprintf(os.Stderr) closure. The type has no
// io.Writer field, so it is not an output facade — queue.Config is this
// shape in the wild. Do not "fix" the Fprintf; the test pins it.
package defaults

import (
	"fmt"
	"os"
)

// Config holds an optional log func, not a wrapped stdout/stderr pair.
type Config struct {
	Logf func(format string, args ...any)
}

func (c *Config) defaults() {
	if c.Logf == nil {
		c.Logf = func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, format, args...)
		}
	}
}
