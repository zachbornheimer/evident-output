package project

import (
	"errors"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// failedPermissions is a Task whose permissions attribute failed with
// failure, carrying that failure on the detail the way File records it.
func failedPermissions(failure error, p record.Problem) record.TaskSnapshot {
	return record.TaskSnapshot{
		ID: "task_1", Name: "install hook", State: record.Failed, Summary: "hook not installed",
		Problems: []record.Problem{p},
		Verification: []record.VerificationDetail{
			{Name: "contents", Status: record.VerificationSatisfied},
			{
				Name: "permissions", Status: record.VerificationError, Failure: failure,
				Facts: []record.Fact{{Name: "error", Value: "operation not permitted"}},
			},
		},
	}
}

func renderTask(t record.TaskSnapshot) string {
	var b strings.Builder
	WriteTask(&b, t, Style{})
	return b.String()
}

// TestProblemDetailSurvivesWrappedAttributeError pins that a failed
// attribute is stated once, by its own branch, whatever text the caller
// wrapped around its failure, and that the caller's own words survive: a
// Detail line that wraps the attribute's failure ("install: failed:
// permissions") keeps its context, a line that is the failure alone is
// dropped, and a line that only looks like the failure on a Task whose
// attribute failed differently is the caller's text and stays.
func TestProblemDetailSurvivesWrappedAttributeError(t *testing.T) {
	failure := errors.New("failed: permissions")
	for _, c := range []struct {
		name    string
		failure error
		detail  string
		want    string
	}{
		{"bare failure is dropped", failure, "failed: permissions", ""},
		{"wrapped failure keeps its context", failure, "install: failed: permissions", "install"},
		{"doubly wrapped failure keeps its context", failure, "hook: install: failed: permissions", "hook: install"},
		{"unrelated line stays", failure, "try sudo", "try sudo"},
		{"the caller's own words stay beside the failure", failure, "try sudo\ninstall: failed: permissions", "try sudo\ninstall"},
		{"a look-alike line stays when the attribute has no such failure", errors.New("chmod refused"), "failed: permissions", "failed: permissions"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := record.Problem{Summary: "hook not installed", Detail: c.detail}
			got := record.Problem{}
			for _, shown := range (TaskRow{t: failedPermissions(c.failure, p)}).visibleProblems("hook not installed", failedPermissions(c.failure, p).Verification) {
				got = shown
			}
			if got.Detail != c.want {
				t.Errorf("Detail = %q, want %q\n%s", got.Detail, c.want, renderTask(failedPermissions(c.failure, p)))
			}
		})
	}
}
