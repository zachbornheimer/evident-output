package apisurface

import "strings"

// Ident is the vocabulary name on one Walk/golden line: "type Foo" and
// "value Foo" (or "value Foo Type") and "func Foo(" become Foo; "func (Bar) Baz(" and
// "type Bar.Baz" become Bar.Baz.
func Ident(line string) string {
	line = strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(line, "type "):
		return strings.TrimSpace(strings.TrimPrefix(line, "type "))
	case strings.HasPrefix(line, "value "):
		name, _, _ := strings.Cut(strings.TrimSpace(strings.TrimPrefix(line, "value ")), " ")
		return name
	case strings.HasPrefix(line, "func ("):
		rest := strings.TrimPrefix(line, "func (")
		typ, rest, ok := strings.Cut(rest, ")")
		if !ok {
			return ""
		}
		name, _, _ := strings.Cut(strings.TrimSpace(rest), "(")
		if typ == "" || name == "" {
			return ""
		}
		return typ + "." + name
	case strings.HasPrefix(line, "func "):
		name, _, _ := strings.Cut(strings.TrimPrefix(line, "func "), "(")
		return strings.TrimSpace(name)
	default:
		return ""
	}
}
