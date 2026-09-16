// Fixture: §62 baseline. Group for independent work, Sequence for ordered
// work, no explicit .After(...) chain — zero EVO-* findings.
package groupseq

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

type Package struct{ Name string }

func run(required []Package) {
	packages := evo.Group("packages")
	for _, pkg := range required {
		pkg := pkg
		task := packages.Task(pkg.Name)
		task.Define(func(ctx context.Context) error {
			return installPackage(ctx, pkg)
		})
	}

	setup := evo.Sequence("launch agent")
	write := setup.Task("write plist")
	write.Define(writePlist)
	register := setup.Task("register")
	register.Define(registerAgent)
}

func installPackage(ctx context.Context, pkg Package) error { return nil }
func writePlist(ctx context.Context) error                  { return nil }
func registerAgent(ctx context.Context) error               { return nil }
