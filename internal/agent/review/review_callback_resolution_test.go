package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// API-040 (E-118 lane B): the shape API-036 used to rewrite — a bare
// task.Fail(fmt.Sprintf(...)) statement inside a Define callback,
// immediately followed by a non-nil `return err` — is not orphaned by
// API-036's removal (CHANGELOG: "API-036 ... removed rather than kept as a
// can-never-clear finding"). Define's own returned-error resolution already
// fails the task, so the explicit Fail call is redundant ceremony; API-040
// is the rule that now catches it and rewrites it down to the bare return.

const failFmtSprintfThenReturnErrInDefineSrc = `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func run(task *evo.TaskHandle) {
  task.Define(func(ctx context.Context) error {
    err := validate()
    if err != nil {
      task.Fail(fmt.Sprintf("validate policy manifest: %s", err))
      return err
    }
    return nil
  })
}
func validate() error { return nil }
`

func TestAPI040_FailFmtSprintfThenReturnErrInDefine_Fires(t *testing.T) {
	res := review.GoSource("policy.go", failFmtSprintfThenReturnErrInDefineSrc)
	f := findingByID(t, res, "API-040")
	if f.Suggestion == "" {
		t.Fatalf("API-040 finding has no suggestion")
	}
}
