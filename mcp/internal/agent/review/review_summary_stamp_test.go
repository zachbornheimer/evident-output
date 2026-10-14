package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// API-060 (1.1/ZYS-971): Task.Summary/Group.Summary is the
// settled row's headline for the caller's own result, not a place to restate —
// a Summary whose text narrates a mutation ("wrote ..."), a dry-run
// hypothetical ("would add ..."), a no-op ("nothing to write"), or an
// already-satisfied precondition ("already ...") belongs to
// evo.File/evo.Effect/the Satisfied outcome/evo.Fact instead.

const summaryStampWroteSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(ctx context.Context, task *evo.Task, path string, contents []byte) error {
  if err := evo.File(ctx, evo.FileSpec{Path: path, Contents: contents}); err != nil {
    return err
  }
  task.Summary("wrote config.json")
  return nil
}
`

func TestAPI060_SummaryWroteNarration_Fires(t *testing.T) {
	res := review.GoSource("stamp_wrote.go", summaryStampWroteSrc)
	f := findingByID(t, res, "API-060")
	if f.Severity != "warning" {
		t.Fatalf("API-060 severity = %q, want warning", f.Severity)
	}
}

const summaryStampWouldAddSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(task *evo.Task) {
  task.Summary("would add 3 refs")
}
`

func TestAPI060_SummaryWouldAddNarration_Fires(t *testing.T) {
	res := review.GoSource("stamp_would.go", summaryStampWouldAddSrc)
	findingByID(t, res, "API-060")
}

const summaryStampNothingToWriteSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(task *evo.Task) {
  task.Summary("nothing to write")
}
`

func TestAPI060_SummaryNothingToWriteNarration_Fires(t *testing.T) {
	res := review.GoSource("stamp_nothing.go", summaryStampNothingToWriteSrc)
	findingByID(t, res, "API-060")
}

const summaryStampAlreadySrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(task *evo.Task) {
  task.Summary("already up to date")
}
`

func TestAPI060_SummaryAlreadyNarration_Fires(t *testing.T) {
	res := review.GoSource("stamp_already.go", summaryStampAlreadySrc)
	findingByID(t, res, "API-060")
}

// A genuine result-metadata Summary — a count, not stamp narration — must
// stay silent (the worked example straight from ZYS-971's Decisions).

const summaryResultMetadataGoodSrc = `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(branches *evo.Task, n int) {
  branches.Summary(fmt.Sprintf("%d checked", n))
}
`

func TestAPI060_SummaryResultMetadataGoodCode_StaysSilent(t *testing.T) {
	res := review.GoSource("stamp_good.go", summaryResultMetadataGoodSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-060" {
			t.Fatalf("false positive API-060 on a plain result-metadata Summary: %+v", f)
		}
	}
}

// A literal that merely uses "already" mid-sentence in an unrelated way is
// still flagged (Decisions names "already ..." as an unqualified example),
// but a Group.Summary carrying the same stamp text is caught by the same
// rule as Task.Summary.

const groupSummaryStampSrc = `package p
import evo "github.com/zachbornheimer/evident-output/v2"
func run(group *evo.Group) {
  group.Summary("already installed")
}
`

func TestAPI060_GroupSummaryStampNarration_Fires(t *testing.T) {
	res := review.GoSource("stamp_group.go", groupSummaryStampSrc)
	findingByID(t, res, "API-060")
}

func TestAPI060_PreOneOneOnePin_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("stamp_wrote.go", summaryStampWroteSrc, "1.0.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-060" {
			t.Fatalf("API-060 fired for a pin older than 1.1.0 (the rule starts at 1.1.0): %+v", f)
		}
	}
}

func TestAPI060_RecheckAfterRemediation_FindingDisappears(t *testing.T) {
	res := review.GoSource("stamp_wrote.go", summaryStampWroteSrc)
	findingByID(t, res, "API-060")

	after := review.GoSource("stamp_good.go", summaryResultMetadataGoodSrc)
	for _, f := range after.Findings {
		if f.RuleID == "API-060" {
			t.Fatalf("API-060 still fires after remediation to result metadata: %+v", f)
		}
	}
}

// Summary on a type that is not an evo Run, Task, Group or Sequence is that
// type's own method, not the row headline.
func TestAPI060_SummaryOnUnrelatedReceiver_StaysSilent(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output/v2"
type report struct{}
func (report) Summary(string) {}
func run(r *evo.Run, rep report) {
  _ = r
  rep.Summary("wrote 3 rows")
}
`
	for _, f := range review.GoSource("unrelated.go", src).Findings {
		if f.RuleID == "API-060" {
			t.Fatalf("API-060 fired on a non-evo Summary method: %+v", f)
		}
	}
}
