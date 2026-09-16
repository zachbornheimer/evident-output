package wire

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/wireschema"
)

const testEvoVersion = "1.0.0"

var (
	testStart  = time.Date(2026, 9, 15, 20, 0, 0, 0, time.UTC)
	testFinish = time.Date(2026, 9, 15, 20, 0, 2, 147000000, time.UTC)
)

func baseConclusion() core.Conclusion {
	return core.Conclusion{
		RunID:      "out_1",
		StartedAt:  testStart,
		FinishedAt: testFinish,
	}
}

// runFixture returns the eight §35-required scenarios: success,
// already-satisfied, failure, blocked, cancelled, dry-run, provenance
// revalidation placeholder, partial effects.
func runFixtures() map[string]core.Result {
	return map[string]core.Result{
		"success": {Conclusion: withConc(func(c *core.Conclusion) {
			c.State = core.StateChanged
			c.Changed = true
			c.ExitCode = core.ExitOK
			c.Tasks = []core.TaskSnapshot{
				core.NewTaskSnapshot(core.TaskSnapshot{
					ID: "task_1", Key: "build", Name: "build", State: core.Done,
					Resolution: core.ResolutionExecuted,
				}, time.Time{}, false, false),
			}
		})},
		"already_satisfied": {Conclusion: withConc(func(c *core.Conclusion) {
			c.State = core.StateReady
			c.ExitCode = core.ExitOK
			c.Tasks = []core.TaskSnapshot{
				core.NewTaskSnapshot(core.TaskSnapshot{
					ID: "task_1", Key: "install", Name: "install", State: core.Done,
					Resolution: core.ResolutionAlreadySatisfied,
					Evidence: core.TaskEvidence{
						Before: core.EvidencePhase{Evaluated: true, Satisfied: true, Source: "verify"},
					},
				}, time.Time{}, false, false),
			}
		})},
		"failure": {Conclusion: withConc(func(c *core.Conclusion) {
			c.State = core.StateFailed
			c.ExitCode = core.ExitFailed
			c.Tasks = []core.TaskSnapshot{
				core.NewTaskSnapshot(core.TaskSnapshot{
					ID: "task_1", Key: "deploy", Name: "deploy", State: core.Failed,
					Problems: []core.Problem{{Code: "deploy.write", Summary: "failed to write"}},
				}, time.Time{}, false, false),
			}
		})},
		"blocked": {Conclusion: withConc(func(c *core.Conclusion) {
			c.State = core.StateBlocked
			c.ExitCode = core.ExitBlocked
			c.Tasks = []core.TaskSnapshot{
				core.NewTaskSnapshot(core.TaskSnapshot{
					ID: "task_1", Key: "confirm", Name: "confirm", State: core.Blocked,
				}, time.Time{}, false, false),
			}
		})},
		"cancelled": {Conclusion: withConc(func(c *core.Conclusion) {
			c.State = core.StateCancelled
			c.Cancelled = true
			c.ExitCode = core.ExitCancelled
			c.Tasks = []core.TaskSnapshot{
				core.NewTaskSnapshot(core.TaskSnapshot{
					ID: "task_1", Key: "scan", Name: "scan", State: core.Done,
				}, time.Time{}, false, false),
				core.NewTaskSnapshot(core.TaskSnapshot{
					ID: "task_2", Key: "venv", Name: "venv", State: core.Cancelled,
				}, time.Time{}, false, false),
			}
		})},
		"dry_run": {Conclusion: withConc(func(c *core.Conclusion) {
			c.State = core.StatePlanned
			c.DryRun = true
			c.ExitCode = core.ExitOK
			c.Plans = []core.PlanSnapshot{
				{ID: "plan_1", Subject: "repo", Records: []core.EffectRecord{
					{Verb: "delete", Object: "stale tag", HasQty: true, Quantity: 3},
				}},
			}
		})},
		"provenance_revalidation": {Conclusion: withConc(func(c *core.Conclusion) {
			c.State = core.StateChanged
			c.Changed = true
			c.ExitCode = core.ExitOK
			c.Tasks = []core.TaskSnapshot{
				core.NewTaskSnapshot(core.TaskSnapshot{
					ID: "task_1", Key: "producer", Name: "producer", State: core.Done,
					Resolution: core.ResolutionExecuted,
					Evidence: core.TaskEvidence{
						Before: core.EvidencePhase{Evaluated: false},
						After:  core.EvidencePhase{Evaluated: true, Satisfied: true, Source: "operations"},
					},
				}, time.Time{}, false, false),
			}
		})},
		"partial_effects": {Conclusion: withConc(func(c *core.Conclusion) {
			c.State = core.StateChanged
			c.Changed = true
			c.Partial = true
			c.ExitCode = core.ExitOK
			c.Changes = []core.ChangesSnapshot{
				{ID: "ch_1", Subject: "repo", Records: []core.EffectRecord{
					{Verb: "deleted", Object: "branch", HasQty: true, Quantity: 1},
				}},
			}
			c.Plans = []core.PlanSnapshot{
				{ID: "plan_1", Subject: "repo", Records: []core.EffectRecord{
					{Verb: "delete", Object: "tag", HasQty: true, Quantity: 2},
				}},
			}
		})},
	}
}

func withConc(mutate func(*core.Conclusion)) core.Conclusion {
	c := baseConclusion()
	mutate(&c)
	return c
}

func TestToRunDocument_Goldens(t *testing.T) {
	for name, result := range runFixtures() {
		t.Run(name, func(t *testing.T) {
			got, err := EncodeRun(result, testEvoVersion)
			if err != nil {
				t.Fatalf("EncodeRun(%s): %v", name, err)
			}
			goldenPath := filepath.Join("testdata", name+".golden.json")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.WriteFile(goldenPath, append(got, '\n'), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden %s: %v (run with UPDATE_GOLDEN=1 to create)", goldenPath, err)
			}
			if string(got)+"\n" != string(want) {
				t.Fatalf("evo.run document for %q does not match golden %s\ngot:\n%s\nwant:\n%s", name, goldenPath, got, want)
			}
		})
	}
}

func TestToRunDocument_ValidatesAgainstSchema(t *testing.T) {
	schema, err := os.ReadFile("../../schema/run.v2.json")
	if err != nil {
		t.Fatalf("read schema/run.v2.json: %v", err)
	}
	for name, result := range runFixtures() {
		t.Run(name, func(t *testing.T) {
			doc, err := EncodeRun(result, testEvoVersion)
			if err != nil {
				t.Fatalf("EncodeRun: %v", err)
			}
			if err := wireschema.Validate(schema, doc); err != nil {
				t.Fatalf("evo.run document for %q does not conform to schema/run.v2.json:\n%v\n\ndocument:\n%s", name, err, doc)
			}
		})
	}
}

func TestOutcomeFor(t *testing.T) {
	cases := []struct {
		state core.ConclusionState
		want  string
	}{
		{core.StateReady, OutcomeOK},
		{core.StateChanged, OutcomeOK},
		{core.StateWarning, OutcomeOK},
		{core.StatePlanned, OutcomeOK},
		{core.StateBlocked, OutcomeBlocked},
		{core.StateFailed, OutcomeFailed},
		{core.StateCancelled, OutcomeCancelled},
	}
	for _, tc := range cases {
		if got := outcomeFor(tc.state); got != tc.want {
			t.Errorf("outcomeFor(%s) = %s, want %s", tc.state, got, tc.want)
		}
	}
}

func TestModeFor(t *testing.T) {
	if got := modeFor(true); got != ModeDryRun {
		t.Errorf("modeFor(true) = %s, want %s", got, ModeDryRun)
	}
	if got := modeFor(false); got != ModeApply {
		t.Errorf("modeFor(false) = %s, want %s", got, ModeApply)
	}
}

func TestToRunDocument_EvidenceOmitsAfterWhenBeforeSatisfied(t *testing.T) {
	result := core.Result{Conclusion: withConc(func(c *core.Conclusion) {
		c.Tasks = []core.TaskSnapshot{
			core.NewTaskSnapshot(core.TaskSnapshot{
				ID: "task_1", Name: "install", State: core.Done,
				Resolution: core.ResolutionAlreadySatisfied,
				Evidence: core.TaskEvidence{
					Before: core.EvidencePhase{Evaluated: true, Satisfied: true, Source: "verify"},
				},
			}, time.Time{}, false, false),
		}
	})}
	doc := ToRunDocument(result, testEvoVersion)
	raw, err := json.Marshal(doc.Data.Tasks[0].Evidence)
	if err != nil {
		t.Fatalf("marshal evidence: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}
	if _, present := m["after"]; present {
		t.Fatalf("evidence.after must be omitted when before.satisfied=true, got %s", raw)
	}
}

func TestToRunDocument_UnevaluatedPhaseOmitsSatisfiedAndSource(t *testing.T) {
	phase := EvidencePhaseDoc{Evaluated: false}
	raw, err := json.Marshal(phase)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) != `{"evaluated":false}` {
		t.Fatalf("unevaluated phase = %s, want {\"evaluated\":false}", raw)
	}
}
