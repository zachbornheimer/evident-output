package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/agent/review"
)

func TestGoSource_DetectsStartPrintfExitAndDetailMisuse(t *testing.T) {
	src := `package p
import (
  "fmt"
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func f() {
  out := evo.New()
  t := out.Task("x")
  t.Start()
  fmt.Printf("hi")
  out.Item("i").Block("b", evo.Detail(err))
  os.Exit(1)
}
`
	res := review.GoSource("x.go", src)
	want := map[string]bool{"API-006": false, "STREAM-003": false, "DOM-014": false, "API-018": false}
	for _, f := range res.Findings {
		if _, ok := want[f.RuleID]; ok {
			want[f.RuleID] = true
			if f.Line == 0 && f.RuleID == "API-006" {
				t.Errorf("API-006 missing line")
			}
		}
	}
	for id, ok := range want {
		if !ok {
			t.Errorf("missing finding %s in %#v", id, res.Findings)
		}
	}
	if !res.RecheckRequired {
		t.Fatal("expected recheck_required")
	}
}

func TestTranscript_CursorHideShow(t *testing.T) {
	res := review.Transcript("t.txt", "\x1b[?25l hello")
	if len(res.Findings) == 0 {
		t.Fatal("expected TERM-008")
	}
}

func TestStructuredDocument_RequiresSchema(t *testing.T) {
	res := review.StructuredDocument("x.json", []byte(`{"foo":1}`))
	if !res.RecheckRequired {
		t.Fatal("expected schema findings")
	}
}
