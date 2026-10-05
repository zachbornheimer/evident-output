package find

import "fmt"

func pointerKey(scope any) string { return fmt.Sprintf("%p", scope) }
