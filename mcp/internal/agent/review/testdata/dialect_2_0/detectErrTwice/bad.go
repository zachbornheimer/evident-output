package fixture

import evo "github.com/zachbornheimer/evident-output/v2"

func validate(err error) error {
	return evo.Fail(err.Error(), evo.Detail(err.Error()))
}
