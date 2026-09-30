// Package fixture is the hermetic stand-in for zq's repository layer. Every
// call answers from memory.
package fixture

import "context"

// Repositories lists the repositories under management.
func Repositories(context.Context) ([]string, error) { return []string{"alpha", "beta"}, nil }

// Adopt brings one repository under management.
func Adopt(context.Context, string) error { return nil }
