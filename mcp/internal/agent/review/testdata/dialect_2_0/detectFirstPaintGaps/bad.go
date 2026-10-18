package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// main reads the config before Main arms the display, so nothing paints yet.
func main() {
	_, _ = os.ReadFile("config.toml")
	os.Exit(evo.Main(evo.Config{}, run))
}

// run reads the file before it declares its first Task.
func run(ctx context.Context, r *evo.Run) error {
	data, err := os.ReadFile("zq.toml")
	if err != nil {
		return evo.Fail("cannot read zq.toml")
	}
	r.Task("scan sources").Define(func(ctx context.Context) error { return scan(data) })
	return nil
}

func scan(data []byte) error { return nil }
