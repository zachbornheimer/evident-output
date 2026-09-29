package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// TestDiscardedMainCode pins E-114/EVO-EXIT-002: evo.Main returns the exit
// code and never exits itself, so a bare `evo.Main(run)` statement (or
// `_ = evo.Main(run)`) exits 0 on a failed or blocked run. Review must flag
// it and suggest os.Exit(evo.Main(run)); the wrapped form stays clean.
func TestDiscardedMainCode(t *testing.T) {
	const head = `package main
import (
	"context"
	"os"
	evo "github.com/zachbornheimer/evident-output"
)
var _ = os.Exit
func run(ctx context.Context) error {
	evo.Task("refuse").Block("disk 100% full")
	return nil
}
func main() {
	evo.Init(evo.Config{Title: "tool"})
`
	for name, tc := range map[string]struct {
		body string
		want bool
	}{
		"bare statement":  {"\tevo.Main(run)\n}\n", true},
		"blank assign":    {"\t_ = evo.Main(run)\n}\n", true},
		"os.Exit wrapped": {"\tos.Exit(evo.Main(run))\n}\n", false},
		"code variable":   {"\tcode := evo.Main(run)\n\tos.Exit(code)\n}\n", false},
	} {
		res := review.GoSource("main.go", head+tc.body)
		var got *review.Finding
		for i := range res.Findings {
			if res.Findings[i].RuleID == "EVO-EXIT-002" {
				got = &res.Findings[i]
			}
		}
		if (got != nil) != tc.want {
			t.Errorf("%s: EVO-EXIT-002 fired=%v, want %v: %+v", name, got != nil, tc.want, res.Findings)
			continue
		}
		if got != nil {
			if got.Suggestion == "" || got.Line != 14 {
				t.Errorf("%s: finding = %+v, want line 14 with a suggestion", name, *got)
			}
			if !res.RecheckRequired {
				t.Errorf("%s: recheck_required=false on a discarded exit code", name)
			}
		}
	}
}
