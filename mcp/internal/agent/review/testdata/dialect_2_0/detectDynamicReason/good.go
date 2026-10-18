package fixture

import evo "github.com/zachbornheimer/evident-output/v2"

const reasonProtected = "protected"

func skip() error {
	return evo.Exclude(reasonProtected)
}
