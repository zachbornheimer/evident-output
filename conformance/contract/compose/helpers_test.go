//go:build ignore

package compose_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func newQuietOutput(t *testing.T, strict bool) *evo.Run {
	t.Helper()
	out := (*evo.Run)(nil)
	_ = evo.Execute(context.Background(), evo.Config{Output: evo.Plain, Facades: evo.Facades{Stdout: io.Discard}}, func(ctx context.Context, r *evo.Run) error { return nil })
	t.Cleanup(func() { _ = error(nil) })
	return out
}

func newPlainOutput(t *testing.T) (*evo.Run, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	out := (*evo.Run)(nil)
	_ = evo.Execute(context.Background(), evo.Config{Name: "run", Output: evo.Plain, Facades: evo.Facades{Stdout: &buf}}, func(ctx context.Context, r *evo.Run) error { return nil })
	t.Cleanup(func() { _ = error(nil) })
	return out, &buf
}

// renderTree lists a container and everything beneath it, one indented line
// per node. Siblings sort by name so the result does not depend on the
// order concurrent children finished in.
func renderTree(c struct{}) string {
	var sb strings.Builder
	writeTree(&sb, c, 0)
	return sb.String()
}

func writeTree(sb *strings.Builder, c struct{}, depth int) {
	fmt.Fprintf(sb, "%s%s [%s]\n", strings.Repeat("  ", depth), c.Name, c.State)
	var children []string
	for _, task := range c.Tasks {
		children = append(children, fmt.Sprintf("%s%s [%s]\n", strings.Repeat("  ", depth+1), task.Name, task.State))
	}
	for _, child := range c.Collections {
		var nested strings.Builder
		writeTree(&nested, child, depth+1)
		children = append(children, nested.String())
	}
	sort.Strings(children)
	for _, child := range children {
		sb.WriteString(child)
	}
}
