package fixture

import (
	"encoding/json"
	"fmt"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func emit(r *evo.Run, doc map[string]string) error {
	if err := json.NewEncoder(r.ResultWriter()).Encode(doc); err != nil {
		return err
	}
	fmt.Println("done")
	return nil
}
