package fixture

import (
	"strings"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func skip(names []string) error {
	return evo.Exclude(strings.Join(names, ", "))
}
