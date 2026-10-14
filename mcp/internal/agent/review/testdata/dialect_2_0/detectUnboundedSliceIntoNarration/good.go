package fixture

import (
	"strings"

	evo "github.com/zachbornheimer/evident-output/v2"
)

const shownNames = 8

func refuse(names []string) error {
	return evo.Fail("cannot delete", evo.Detail(strings.Join(names[:min(len(names), shownNames)], ", ")))
}
