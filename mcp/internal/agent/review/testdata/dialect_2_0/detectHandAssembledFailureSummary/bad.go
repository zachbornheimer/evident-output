package fixture

import (
	"fmt"
	"strings"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func report(r *evo.Run, failures []string) {
	fmt.Fprintln(r.Text(), strings.Join(failures, "\n"))
}
