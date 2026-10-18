package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

type Package struct{ Name string }

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	return installAll(r.Group("packages"), []Package{{Name: "core"}, {Name: "cli"}})
}

func installAll(group *evo.Group, pkgs []Package) error {
	for _, pkg := range pkgs {
		group.Task("install " + pkg.Name).Define(func(ctx context.Context) error { return install(ctx, pkg) })
	}
	return nil
}

func install(ctx context.Context, pkg Package) error { return nil }
