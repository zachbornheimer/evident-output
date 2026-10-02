// Fixture: EVO-UI-004 must fire twice, once for a hand-picked color and once
// for a bracketed status word.
package ui004

import "fmt"

func report(name string) {
	fmt.Print("\x1b[32mdone\x1b[0m ", name, "\n")
	fmt.Println("[FAIL]", name)
}
