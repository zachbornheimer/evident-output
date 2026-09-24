package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// API-062 (1.2/ZYS-945): Evo stamps every Task's lifecycle and derives the
// run aggregate (TaskSnapshot.Timing, Conclusion.Metrics, JSON timing and
// data.metrics, the Verbose timing line). A caller stopwatch fed into a
// Task's Summary or Fact hand-rolls that truth as unstructured prose.

const manualTimingSummarySrc = `package p
import (
  "fmt"
  "time"
  evo "github.com/zachbornheimer/evident-output"
)
func run(task *evo.TaskHandle) {
  start := time.Now()
  task.Define(func(ctx context.Context) error {
    err := build(ctx)
    task.Summary(fmt.Sprintf("built in %s", time.Since(start)))
    return err
  })
}
`

func TestAPI062_StopwatchInSummary_Fires(t *testing.T) {
	res := review.GoSource("timing_summary.go", manualTimingSummarySrc)
	f := findingByID(t, res, "API-062")
	if f.Severity != "warning" || f.RequiredVersion != "1.2.0" {
		t.Fatalf("API-062 = %+v, want a warning requiring 1.2.0", f)
	}
}

const manualTimingFactSrc = `package p
import (
  "time"
  evo "github.com/zachbornheimer/evident-output"
)
func run(task *evo.TaskHandle, start time.Time) {
  task.Fact("duration", time.Now().Sub(start).String())
}
`

func TestAPI062_ClockSubtractionInTaskFact_Fires(t *testing.T) {
	res := review.GoSource("timing_fact.go", manualTimingFactSrc)
	findingByID(t, res, "API-062")
}

const manualTimingRunFactSrc = `package p
import (
  "time"
  evo "github.com/zachbornheimer/evident-output"
)
func run(start time.Time) {
  evo.Fact("elapsed", time.Since(start).String())
}
`

func TestAPI062_StopwatchInRunFact_Fires(t *testing.T) {
	res := review.GoSource("timing_run_fact.go", manualTimingRunFactSrc)
	findingByID(t, res, "API-062")
}

// A stopwatch that drives domain logic (a deadline) and never reaches a
// Summary/Fact is not timing narration; neither is a count Summary.
const manualTimingDomainDeadlineSrc = `package p
import (
  "fmt"
  "time"
  evo "github.com/zachbornheimer/evident-output"
)
func run(task *evo.TaskHandle, start time.Time, n int) error {
  if time.Since(start) > time.Minute {
    return errDeadline
  }
  task.Summary(fmt.Sprintf("%d checked", n))
  return nil
}
`

func TestAPI062_DeadlineAndCountSummary_Silent(t *testing.T) {
	res := review.GoSource("timing_deadline.go", manualTimingDomainDeadlineSrc)
	assertNoFinding(t, res, "API-062")
}

func TestAPI062_PinBeforeOneTwo_Silent(t *testing.T) {
	res := review.GoSourceAt("timing_summary.go", manualTimingSummarySrc, "1.1.0")
	assertNoFinding(t, res, "API-062")
}
