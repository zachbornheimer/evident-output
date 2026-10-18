package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func declare(r *evo.Run, notes []string) {
	for _, note := range notes {
		r.Task(note).Define(func(context.Context) error {
			return evo.Exclude(note)
		})
	}
}
