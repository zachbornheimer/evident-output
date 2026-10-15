package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// writeReport runs the render as an Exec with declared Inputs and Outputs,
// so Evo skips it when the report is current.
func writeReport(ctx context.Context, output string, sources []string) error {
	_, err := evo.Exec(ctx, evo.ExecSpec{
		Argv:    append([]string{"render-report", output}, sources...),
		Inputs:  sources,
		Outputs: []string{output},
	})
	return err
}
