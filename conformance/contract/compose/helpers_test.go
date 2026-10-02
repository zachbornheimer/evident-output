package compose_test

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func newQuietOutput(t *testing.T, strict bool) *evo.Output {
	t.Helper()
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Plain: true, Strict: strict})
	t.Cleanup(func() { _ = out.Close() })
	return out
}

func newPlainOutput(t *testing.T) (*evo.Output, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Plain: true, Color: evo.ColorNever, Title: "run"})
	t.Cleanup(func() { _ = out.Close() })
	return out, &buf
}

// renderTree lists a container and everything beneath it, one indented line
// per node. Siblings sort by name so the result does not depend on the
// order concurrent children finished in.
func renderTree(c evo.TasksSnapshot) string {
	var sb strings.Builder
	writeTree(&sb, c, 0)
	return sb.String()
}

func writeTree(sb *strings.Builder, c evo.TasksSnapshot, depth int) {
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
