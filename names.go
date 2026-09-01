package evo

import (
	"fmt"
	"strings"
)

// DefaultVisibleNames is how many names TruncateNames keeps before summarizing.
const DefaultVisibleNames = 3

// TruncateNames joins names for a skip/kept-style summary.
// Empty names yields "". visible <= 0 uses DefaultVisibleNames.
// When more names remain than visible, appends ", +N".
func TruncateNames(names []string, visible int) string {
	if len(names) == 0 {
		return ""
	}
	if visible <= 0 {
		visible = DefaultVisibleNames
	}
	if len(names) <= visible {
		return strings.Join(names, ", ")
	}
	shown := names[:visible]
	omitted := len(names) - visible
	return strings.Join(shown, ", ") + fmt.Sprintf(", +%d", omitted)
}
