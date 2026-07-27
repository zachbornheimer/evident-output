// Example: doctor/check command — many independent Items, mixed outcomes.
package main

import (
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	out := evo.For("env-doctor", evo.To(os.Stdout), evo.Plain(), evo.NoColor())
	defer out.Close()

	out.Item("go toolchain").OK()
	out.Item("mise tasks").OK()
	out.Item("git signed commits").Warn(
		"signing not verified in this demo",
		evo.Detail("run: git config commit.gpgsign true"),
	)
	out.Item("disk free space").Block(
		"less than 2 GiB free on /",
		evo.Detail("free space before large builds"),
	).Because("CI images fail when the volume fills mid-test.")
	out.Item("docker daemon").Fail(
		"cannot connect to docker socket",
		evo.Detail("start Colima or Docker Desktop"),
	)

	if err := out.Finish(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		// Finish misuse is separate from presentation conclusion.
	}
	os.Exit(out.Conclusion().ExitCode)
}
