// Example API-013: host-framework wiring shapes without adding urfave/cli or
// Kong as core dependencies. Real apps plug evo into Action/Run handlers.
//
//	// urfave/cli v2 shape
//	&cli.Command{Name: "status", Action: statusAction}
//
//	// Kong shape
//	type StatusCmd struct{}
//	func (c *StatusCmd) Run() error { return runStatus() }
package main

import (
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

// statusAction is the urfave/cli-shaped entry: (*cli.Context) error without
// importing urfave — signature is what adapters call.
func statusAction(/* ctx *cli.Context */) error {
	return runStatus()
}

// StatusCmd.Run is the Kong-shaped entry.
type StatusCmd struct{}

func (c *StatusCmd) Run() error { return runStatus() }

func runStatus() error {
	out := evo.For("app-status", evo.To(os.Stdout), evo.Plain(), evo.NoColor())
	defer out.Close()
	out.Item("config").OK()
	out.Item("database").OK()
	if err := out.Finish(); err != nil {
		return err
	}
	if code := out.Conclusion().ExitCode; code != 0 {
		os.Exit(code)
	}
	return nil
}

func main() {
	if err := statusAction(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// Kong path compiles the same body:
	_ = (&StatusCmd{}).Run
}
