package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
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
import evo "github.com/zachbornheimer/evident-output"
func f() {
  out := evo.Init(evo.Config{})
  t := out.Task("env")
  t.Printf("go version: %s\n", "1.23.0")
}
`
	res := review.GoSource("x.go", bad)
	if !hasFinding(res, "EVO-UI-001") {
		t.Fatalf("expected EVO-UI-001 on manually printed \"label: value\" line: %+v", res.Findings)
	}

	good := `package p
import evo "github.com/zachbornheimer/evident-output"
func f() {
  out := evo.Init(evo.Config{})
  t := out.Task("env")
  t.Fact("go version", "1.23.0")
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
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func f() {
  out := evo.Init(evo.Config{})
  t := out.Task("check")
  fmt.Println("✓ verified")
  t.Done()
}
`
	res := review.GoSource("x.go", bad)
	if !hasFinding(res, "EVO-UI-002") {
		t.Fatalf("expected EVO-UI-002 on manually printed success line: %+v", res.Findings)
	}

	good := `package p
import evo "github.com/zachbornheimer/evident-output"
func f() {
  out := evo.Init(evo.Config{})
  t := out.Task("check")
  t.Done()
}
`
	res = review.GoSource("x.go", good)
	if hasFinding(res, "EVO-UI-002") {
		t.Fatalf("false positive EVO-UI-002 on plain task.Done: %+v", res.Findings)
	}
}

// TestGoSource_EvoUI002_UnrelatedTextNotFlagged covers ordinary informational
// text that merely contains the substring "verified"/"passed" — not a
// completion confirmation duplicating task.Done's glyph.
func TestGoSource_EvoUI002_UnrelatedTextNotFlagged(t *testing.T) {
	src := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func f() {
  out := evo.Init(evo.Config{})
  t := out.Task("check")
  fmt.Println("license verified against upstream, expires in 30 days")
  fmt.Println("2 hours passed since the last successful run")
  t.Done()
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
  evo "github.com/zachbornheimer/evident-output"
)
func f() {
  out := evo.Init(evo.Config{})
  t := out.Task("copy")
  fmt.Printf("%d/%d done\n", 3, 10)
  t.Done()
}
`
	res := review.GoSource("x.go", bad)
	if !hasFinding(res, "EVO-UI-003") {
		t.Fatalf("expected EVO-UI-003 on hand-built N/M progress text: %+v", res.Findings)
	}

	good := `package p
import evo "github.com/zachbornheimer/evident-output"
func f() {
  out := evo.Init(evo.Config{})
  t := out.Task("copy")
  t.Progress(3, 10)
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
  evo "github.com/zachbornheimer/evident-output"
)
func f() {
  out := evo.Init(evo.Config{})
  t := out.Task("report")
  fmt.Printf("score: %d/%d\n", 7, 10)
  fmt.Printf("date: %d/%d/%d\n", 2024, 1, 15)
  t.Done()
}
`
	res := review.GoSource("x.go", src)
	if hasFinding(res, "EVO-UI-003") {
		t.Fatalf("false positive EVO-UI-003 on unrelated N/M-shaped Printf (score/date): %+v", res.Findings)
	}
}

func TestGoSource_EvoWire001_MarshalOfInternalSnapshot(t *testing.T) {
	bad := `package p
import (
  "encoding/json"
  evo "github.com/zachbornheimer/evident-output"
)
func f() {
  out := evo.Init(evo.Config{})
  b, _ := json.Marshal(out.Snapshot())
  _ = b
}
`
	res := review.GoSource("x.go", bad)
	if !hasFinding(res, "EVO-WIRE-001") {
		t.Fatalf("expected EVO-WIRE-001 on json.Marshal(out.Snapshot()): %+v", res.Findings)
	}

	good := `package p
import (
  evo "github.com/zachbornheimer/evident-output"
  "github.com/zachbornheimer/evident-output/internal/render"
)
func f() {
  out := evo.Init(evo.Config{})
  b, _ := render.EncodeJSON(out.Snapshot())
  _ = b
}
`
	res = review.GoSource("x.go", good)
	if hasFinding(res, "EVO-WIRE-001") {
		t.Fatalf("false positive EVO-WIRE-001 on render.EncodeJSON: %+v", res.Findings)
	}
}

// TestGoSource_EvoWire001_UnrelatedResultReceiverNotFlagged covers a file
// that imports evo (so hasEvo is true) but marshals an unrelated type's own
// .Result() accessor — evo's public API has no .Result() method (Run/
// Output.Run return a Result value directly), so this must never fire.
func TestGoSource_EvoWire001_UnrelatedResultReceiverNotFlagged(t *testing.T) {
	src := `package p
import (
  "encoding/json"
  evo "github.com/zachbornheimer/evident-output"
)
type testRun struct{}
func (r *testRun) Result() string { return "ok" }
func f(r *testRun) {
  _ = evo.Init(evo.Config{})
  b, _ := json.Marshal(r.Result())
  _ = b
}
`
	res := review.GoSource("x.go", src)
	if hasFinding(res, "EVO-WIRE-001") {
		t.Fatalf("false positive EVO-WIRE-001 on an unrelated type's own .Result(): %+v", res.Findings)
	}
}

func TestGoSource_EvoWire003_JSONStdoutMixedWithHumanText(t *testing.T) {
	bad := `package p
import (
  "encoding/json"
  "fmt"
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func f() {
  out := evo.Init(evo.Config{})
  doc := out.Snapshot()
  json.NewEncoder(os.Stdout).Encode(doc)
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
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func f() {
  out := evo.Init(evo.Config{})
  doc := out.Snapshot()
  json.NewEncoder(os.Stdout).Encode(doc)
}
`
	res = review.GoSource("x.go", good)
	if hasFinding(res, "EVO-WIRE-003") {
		t.Fatalf("false positive EVO-WIRE-003 on JSON-only stdout: %+v", res.Findings)
	}
}

func TestGoSource_EvoExit001_OsExitBypassesConclusion(t *testing.T) {
	bad := `package p
import (
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func f() {
  _ = evo.Init(evo.Config{})
  os.Exit(1)
}
`
	res := review.GoSource("x.go", bad)
	if !hasFinding(res, "EVO-EXIT-001") {
		t.Fatalf("expected EVO-EXIT-001 on naked os.Exit(1): %+v", res.Findings)
	}

	good := `package p
import (
  "context"
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func f() {
  os.Exit(evo.Main(func(ctx context.Context) error { return nil }))
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
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func f() {
  _ = evo.Init(evo.Config{})
  fmt.Println("still going...")
}
`
	res := review.GoSource("x.go", bad)
	if !hasFinding(res, "EVO-LIVE-001") {
		t.Fatalf("expected EVO-LIVE-001 on fmt.Println alongside evo: %+v", res.Findings)
	}

	good := `package p
import evo "github.com/zachbornheimer/evident-output"
func f() {
  out := evo.Init(evo.Config{})
  out.Println("still going...")
}
`
	res = review.GoSource("x.go", good)
	if hasFinding(res, "EVO-LIVE-001") {
		t.Fatalf("false positive EVO-LIVE-001 on out.Println: %+v", res.Findings)
	}
}

// TestGoSource_EvoUI00x_UnrelatedReceiverNotFlagged covers a file that
// imports evo (so hasEvo is true) but also calls Print/Printf/Println on
// receivers unrelated to evo's own Task/Output presentation — a stdlib
// log.Printf and a cobra *cobra.Command's cmd.Println/cmd.Printf. None of
// these duplicate an evo Fact/Done/Progress call, so EVO-UI-001/002/003
// must not fire on them merely because their literal text pattern-matches.
func TestGoSource_EvoUI00x_UnrelatedReceiverNotFlagged(t *testing.T) {
	src := `package p
import (
  "log"
  evo "github.com/zachbornheimer/evident-output"
)
func f(cmd *cobra.Command) {
  _ = evo.Init(evo.Config{})
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
