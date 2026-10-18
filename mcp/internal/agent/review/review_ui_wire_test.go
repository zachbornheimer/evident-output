package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// hasFinding reports whether res contains a finding for ruleID.
func hasFinding(res review.Result, ruleID string) bool {
	for _, f := range res.Findings {
		if f.RuleID == ruleID {
			return true
		}
	}
	return false
}

func TestGoSource_EvoUI001_FactPrintedManually(t *testing.T) {
	bad := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run) {
  fmt.Fprintf(r.Text(), "go version: %s\n", "1.23.0")
}
`
	res := review.GoSource("x.go", bad)
	if !hasFinding(res, "EVO-UI-001") {
		t.Fatalf("expected EVO-UI-001 on manually printed \"label: value\" line: %+v", res.Findings)
	}

	good := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(r *evo.Run) {
  r.Task("env").Fact("go version", "1.23.0")
}
`
	res = review.GoSource("x.go", good)
	if hasFinding(res, "EVO-UI-001") {
		t.Fatalf("false positive EVO-UI-001 on task.Fact: %+v", res.Findings)
	}
}

func TestGoSource_EvoUI002_PassingVerificationPrinted(t *testing.T) {
	bad := `package p
import (
  "context"
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run) {
  r.Task("check").Define(func(ctx context.Context) error {
    fmt.Fprintln(r.Text(), "✓ verified")
    return nil
  })
}
`
	res := review.GoSource("x.go", bad)
	if !hasFinding(res, "EVO-UI-002") {
		t.Fatalf("expected EVO-UI-002 on manually printed success line: %+v", res.Findings)
	}

	good := `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run) {
  r.Task("check").Define(func(ctx context.Context) error { return nil })
}
`
	res = review.GoSource("x.go", good)
	if hasFinding(res, "EVO-UI-002") {
		t.Fatalf("false positive EVO-UI-002 on a plain Define: %+v", res.Findings)
	}
}

// TestGoSource_EvoUI002_UnrelatedTextNotFlagged covers ordinary informational
// text that merely contains the substring "verified"/"passed" — not a
// completion confirmation duplicating the Task's success row.
func TestGoSource_EvoUI002_UnrelatedTextNotFlagged(t *testing.T) {
	src := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run) {
  fmt.Fprintln(r.Text(), "license verified against upstream, expires in 30 days")
  fmt.Fprintln(r.Text(), "2 hours passed since the last successful run")
}
`
	res := review.GoSource("x.go", src)
	if hasFinding(res, "EVO-UI-002") {
		t.Fatalf("false positive EVO-UI-002 on ordinary text containing verified/passed: %+v", res.Findings)
	}
}

func TestGoSource_EvoUI003_HandBuiltProgressText(t *testing.T) {
	bad := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run) {
  fmt.Fprintf(r.Text(), "%d/%d done\n", 3, 10)
}
`
	res := review.GoSource("x.go", bad)
	if !hasFinding(res, "EVO-UI-003") {
		t.Fatalf("expected EVO-UI-003 on hand-built N/M progress text: %+v", res.Findings)
	}

	good := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func f(r *evo.Run) {
  r.Task("copy").Progress(3, 10)
}
`
	res = review.GoSource("x.go", good)
	if hasFinding(res, "EVO-UI-003") {
		t.Fatalf("false positive EVO-UI-003 on task.Progress: %+v", res.Findings)
	}
}

// TestGoSource_EvoUI003_UnrelatedNOfMShapeNotFlagged covers "%d/%d"-shaped
// Printf calls that are not progress counts: a score/ratio, and a
// day/month/year date (three %d verbs, so the first two look like an N/M
// shape at a glance).
func TestGoSource_EvoUI003_UnrelatedNOfMShapeNotFlagged(t *testing.T) {
	src := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run) {
  fmt.Fprintf(r.Text(), "score: %d/%d\n", 7, 10)
  fmt.Fprintf(r.Text(), "date: %d/%d/%d\n", 2024, 1, 15)
}
`
	res := review.GoSource("x.go", src)
	if hasFinding(res, "EVO-UI-003") {
		t.Fatalf("false positive EVO-UI-003 on unrelated N/M-shaped Printf (score/date): %+v", res.Findings)
	}
}

func TestGoSource_EvoWire003_JSONStdoutMixedWithHumanText(t *testing.T) {
	bad := `package p
import (
  "encoding/json"
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run, doc map[string]string) {
  json.NewEncoder(r.ResultWriter()).Encode(doc)
  fmt.Println("done")
}
`
	res := review.GoSource("x.go", bad)
	if !hasFinding(res, "EVO-WIRE-003") {
		t.Fatalf("expected EVO-WIRE-003 when human text mixes with JSON stdout: %+v", res.Findings)
	}

	good := `package p
import (
  "encoding/json"
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run, doc map[string]string) {
  json.NewEncoder(r.ResultWriter()).Encode(doc)
  fmt.Fprintln(r.Text(), "done")
}
`
	res = review.GoSource("x.go", good)
	if hasFinding(res, "EVO-WIRE-003") {
		t.Fatalf("false positive EVO-WIRE-003 on payload to ResultWriter and text to Text: %+v", res.Findings)
	}
}

func TestGoSource_EvoExit001_OsExitBypassesConclusion(t *testing.T) {
	bad := `package p
import (
  "os"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f() {
  code := evo.Main(evo.Config{}, run)
  _ = code
  os.Exit(1)
}
`
	res := review.GoSource("x.go", bad)
	if !hasFinding(res, "EVO-EXIT-001") {
		t.Fatalf("expected EVO-EXIT-001 on naked os.Exit(1): %+v", res.Findings)
	}

	good := `package p
import (
  "os"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f() {
  os.Exit(evo.Main(evo.Config{}, run))
}
`
	res = review.GoSource("x.go", good)
	if hasFinding(res, "EVO-EXIT-001") {
		t.Fatalf("false positive EVO-EXIT-001 on os.Exit(evo.Main(run)): %+v", res.Findings)
	}
}

func TestGoSource_EvoLive001_PrintCompetesWithLiveRendering(t *testing.T) {
	bad := `package p
import (
  "context"
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, r *evo.Run) error {
  fmt.Println("still going...")
  return nil
}
`
	res := review.GoSource("x.go", bad)
	if !hasFinding(res, "EVO-LIVE-001") {
		t.Fatalf("expected EVO-LIVE-001 on fmt.Println alongside evo: %+v", res.Findings)
	}

	good := `package p
import (
  "context"
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, r *evo.Run) error {
  fmt.Fprintln(r.Text(), "still going...")
  return nil
}
`
	res = review.GoSource("x.go", good)
	if hasFinding(res, "EVO-LIVE-001") {
		t.Fatalf("false positive EVO-LIVE-001 on fmt.Fprintln(r.Text(), ...): %+v", res.Findings)
	}
}

// TestGoSource_EvoUI00x_UnrelatedReceiverNotFlagged covers a file that
// imports evo (so hasEvo is true) but also prints through writers unrelated
// to Run.Text(): a stdlib log.Printf and a cobra *cobra.Command's
// cmd.Println/cmd.Printf. None of these duplicate an evo Fact or Progress
// call, so EVO-UI-001/002/003 must not fire on them merely because their
// literal text pattern-matches.
func TestGoSource_EvoUI00x_UnrelatedReceiverNotFlagged(t *testing.T) {
	src := `package p
import (
  "log"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(cmd *cobra.Command, r *evo.Run) {
  _ = r
  log.Printf("go version: %s\n", "1.23.0")
  cmd.Println("✓ verified")
  cmd.Printf("%d/%d done\n", 3, 10)
}
`
	res := review.GoSource("x.go", src)
	for _, id := range []string{"EVO-UI-001", "EVO-UI-002", "EVO-UI-003"} {
		if hasFinding(res, id) {
			t.Fatalf("false positive %s on unrelated (log/cmd) receiver: %+v", id, res.Findings)
		}
	}
}

// A line printed straight to stdout is not a Fact: EVO-UI-001 only judges
// text written to Run.Text().
func TestGoSource_EvoUI001_StdoutPayloadLineNotFlagged(t *testing.T) {
	src := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func f(r *evo.Run) {
  _ = r
  fmt.Printf("go version: %s\n", "1.23.0")
}
`
	if res := review.GoSource("x.go", src); hasFinding(res, "EVO-UI-001") {
		t.Fatalf("false positive EVO-UI-001 on a stdout payload line: %+v", res.Findings)
	}
}
