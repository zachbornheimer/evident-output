package fixture

import (
	"context"
	"errors"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// runCaptured lets Exec capture the output and classify the exit.
func runCaptured(ctx context.Context, argv []string) error {
	res, err := evo.Exec(ctx, evo.ExecSpec{Argv: argv})
	if errors.Is(err, evo.ErrExecNonzeroExit) {
		return evo.Fail("command failed", evo.Detail(res.Stderr))
	}
	return err
}
