package review_test

import (
	"slices"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// streamRuleIDs are the rules a stray print beside a live run raises.
var streamRuleIDs = []string{"STREAM-003", "EVO-LIVE-001"}

// streamFindingLines are the lines of the STREAM-003 and EVO-LIVE-001
// findings in src, per rule.
func streamFindingLines(src string) map[string][]int {
	lines := map[string][]int{}
	for _, f := range review.GoSource("main.go", src).Findings {
		for _, id := range streamRuleIDs {
			if f.RuleID == id {
				lines[id] = append(lines[id], f.Line)
			}
		}
	}
	return lines
}

// TestStreamRulesIgnorePrintAfterExecute proves STREAM-003 and EVO-LIVE-001
// guard only a live run: a print after evo.Execute (or evo.Main) has
// returned raises neither, while a print inside the run body, or after a run
// started on another goroutine, still does.
func TestStreamRulesIgnorePrintAfterExecute(t *testing.T) {
	const header = `package main

import (
	"context"
	"fmt"

	evo "github.com/zachbornheimer/evident-output/v2"
)

`
	for _, tc := range []struct {
		name string
		body string
		want map[string][]int // finding lines per rule, header excluded
	}{
		{
			name: "report printed after Execute returned",
			body: `func main() {
	for _, name := range []string{"a", "b"} {
		res := evo.Execute(context.Background(), evo.Config{Name: name}, run)
		fmt.Printf("%s exit=%d\n", name, res.ExitCode())
	}
}
func run(ctx context.Context, r *evo.Run) error { return nil }
`,
			want: nil,
		},
		{
			name: "report printed after Main returned",
			body: `func main() {
	code := evo.Main(evo.Config{}, run)
	fmt.Println("done", code)
}
func run(ctx context.Context, r *evo.Run) error { return nil }
`,
			want: nil,
		},
		{
			name: "print inside the run body",
			body: `func main() { evo.Execute(context.Background(), evo.Config{}, run) }
func run(ctx context.Context, r *evo.Run) error {
	fmt.Println("still going...")
	return nil
}
`,
			want: map[string][]int{"STREAM-003": {12}, "EVO-LIVE-001": {12}},
		},
		{
			name: "print inside a RunFunc literal beside the one after",
			body: `func main() {
	res := evo.Execute(context.Background(), evo.Config{}, func(ctx context.Context, r *evo.Run) error {
		fmt.Println("live")
		return nil
	})
	fmt.Println(res.ExitCode())
}
`,
			want: map[string][]int{"STREAM-003": {12}, "EVO-LIVE-001": {12}},
		},
		{
			name: "print before Execute",
			body: `func main() {
	fmt.Println("starting")
	evo.Execute(context.Background(), evo.Config{}, nil)
}
`,
			want: map[string][]int{"STREAM-003": {11}, "EVO-LIVE-001": {11}},
		},
		{
			name: "print after a run started on a goroutine",
			body: `func main() {
	go evo.Execute(context.Background(), evo.Config{}, nil)
	fmt.Println("the run is still live")
}
`,
			want: map[string][]int{"STREAM-003": {12}, "EVO-LIVE-001": {12}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := streamFindingLines(header + tc.body)
			for _, id := range streamRuleIDs {
				if !slices.Equal(got[id], tc.want[id]) {
					t.Errorf("%s lines = %v, want %v", id, got[id], tc.want[id])
				}
			}
		})
	}
}
