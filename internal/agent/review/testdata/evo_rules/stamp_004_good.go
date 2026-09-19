// Fixture: EVO-STAMP-004 must stay silent. Verb+object Task name.
package stamp004

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func report(out *evo.Output, path string) {
	t := out.Task("check file integrity")
	t.Define(func(ctx context.Context) error { return check(path) })
}

func check(path string) error { return nil }
