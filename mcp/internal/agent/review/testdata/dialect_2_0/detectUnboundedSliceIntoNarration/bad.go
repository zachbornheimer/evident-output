package fixture

import (
	"strings"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func refuse(names []string) error {
	return evo.Fail("cannot delete", evo.Detail(strings.Join(names, ", ")))
}
