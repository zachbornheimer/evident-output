package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// main arms the display first; the config is read inside the first Task.
func main() {
	os.Exit(evo.Main(evo.Config{}, run))
}

func run(ctx context.Context, r *evo.Run) error {
	r.Task("scan sources").Define(func(ctx context.Context) error {
		data, err := os.ReadFile("zq.toml")
		if err != nil {
			return evo.Fail("cannot read zq.toml")
		}
		return scan(data)
	})
	return nil
}

func scan(data []byte) error { return nil }
