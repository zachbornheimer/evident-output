package evaltask_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/evaltask"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

func testdata(t *testing.T) (*evaltask.Grader, []evaltask.Task, []evaltask.Trap) {
	t.Helper()
	fsys := os.DirFS(filepath.Join(repoRoot(t), "internal", "agent", "evaltask", "testdata"))
	tasks, err := evaltask.LoadTasks(fsys)
	if err != nil {
		t.Fatalf("LoadTasks: %v", err)
	}
	traps, err := evaltask.LoadTraps(fsys)
	if err != nil {
		t.Fatalf("LoadTraps: %v", err)
	}
	return &evaltask.Grader{RepoRoot: repoRoot(t), Runner: evaltask.ExecRunner{}}, tasks, traps
}

// Every non-blocked reference compiles, reviews clean, and runs to the
// expected topology; blocked tasks are listed, not silently skipped.
func TestReplay_ReferencesPassEveryGrade(t *testing.T) {
	grader, tasks, _ := testdata(t)
	for _, task := range tasks {
		t.Run(task.ID, func(t *testing.T) {
			if task.Blocked() {
				t.Skipf("blocked on %v", task.Expect.BlockedAPI)
			}
			reference, err := task.ReferenceFS()
			if err != nil {
				t.Fatalf("ReferenceFS: %v", err)
			}
			report, err := grader.Grade(context.Background(), task, reference, t.TempDir())
			if err != nil {
				t.Fatalf("Grade: %v", err)
			}
			assertReferenceReport(t, report)
		})
	}
}

func assertReferenceReport(t *testing.T, report evaltask.Report) {
	t.Helper()
	if !report.Compiles {
		t.Fatalf("reference does not compile:\n%s", report.BuildOutput)
	}
	if !report.ReviewClean {
		t.Errorf("review not clean: %v", report.ReviewFindings)
	}
	if !report.TopologyMatches {
		t.Errorf("topology mismatch: order=%v\n%s", report.OrderViolations, report.TopologyDiff)
	}
	if len(report.BannedPatterns) > 0 || report.Cycles {
		t.Errorf("banned=%v cycles=%v", report.BannedPatterns, report.Cycles)
	}
}

// Every trap is rejected: its banned pattern fires, and review either flags
// it today or the trap is recorded as pending the rule that will.
func TestReplay_TrapsAreRejected(t *testing.T) {
	grader, tasks, traps := testdata(t)
	byID := map[string]evaltask.Task{}
	for _, task := range tasks {
		byID[task.ID] = task
	}
	for _, trap := range traps {
		t.Run(trap.ID, func(t *testing.T) {
			if trap.Blocked() {
				t.Skipf("blocked on %v", trap.BlockedAPI)
			}
			answer, err := trap.AnswerFS()
			if err != nil {
				t.Fatalf("AnswerFS: %v", err)
			}
			report, err := grader.GradeSource(context.Background(), byID[trap.Task], answer, t.TempDir())
			if err != nil {
				t.Fatalf("Grade: %v", err)
			}
			assertTrapRejected(t, trap, report)
		})
	}
}

func assertTrapRejected(t *testing.T, trap evaltask.Trap, report evaltask.Report) {
	t.Helper()
	if !report.Compiles {
		t.Fatalf("trap answer must compile to be a realistic bad answer:\n%s", report.BuildOutput)
	}
	if !slices.Contains(report.BannedPatterns, trap.BannedPattern) {
		t.Errorf("banned pattern %q not detected; fired=%v", trap.BannedPattern, report.BannedPatterns)
	}
	flagged := slices.ContainsFunc(report.ReviewFindings, func(rule string) bool {
		return slices.Contains(trap.ReviewRules, rule)
	})
	switch {
	case trap.ExpectedFlagPending && len(report.ReviewFindings) > 0:
		t.Errorf("review now flags this trap (%v): drop expected_flag_pending and name the rule in review_rules", report.ReviewFindings)
	case !trap.ExpectedFlagPending && !flagged:
		t.Errorf("review rules %v did not fire; findings=%v", trap.ReviewRules, report.ReviewFindings)
	}
}
