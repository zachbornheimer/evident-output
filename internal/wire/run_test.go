package wire

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// TestToRunDocument_VerificationProjectsPerAttributeFactsToo is ZYS-823's
// Patch/File machine-provenance gap: a failing evo.File/Patch attribute's
// Facts (error/path/mode — internal/core's fileVerificationDetails, §8.2)
// are exactly what a machine consumer needs to explain the failure without
// parsing terminal prose, but toTaskDoc hard-coded Verification to an empty
// slice, so WriteJSON's public "evo.run" document dropped every
// VerificationDetail (and its Facts) a Task recorded.
func TestToRunDocument_VerificationProjectsPerAttributeFactsToo(t *testing.T) {
	result := core.Result{Conclusion: withConc(func(c *core.Conclusion) {
		c.Tasks = []core.TaskSnapshot{
			core.NewTaskSnapshot(core.TaskSnapshot{
				ID: "task_1", Name: "write launch agent", State: core.Failed,
				Verification: []core.VerificationDetail{
					{Name: "contents", Status: core.VerificationSatisfied},
					{
						Name: "permissions", Status: core.VerificationError,
						Facts: []core.Fact{
							{Name: "error", Value: "operation not permitted"},
							{Name: "path", Value: "~/Library/LaunchAgents/com.acme.prod.agent.plist"},
							{Name: "mode", Value: "0644"},
						},
					},
				},
			}, time.Time{}, false, false),
		}
	})}
	doc := ToRunDocument(result, testEvoVersion)
	got := doc.Data.Tasks[0].Verification
	want := []VerificationDoc{
		{Name: "contents", Status: VerificationSatisfied},
		{
			Name: "permissions", Status: VerificationError,
			Facts: []FactDoc{
				{Name: "error", Value: "operation not permitted"},
				{Name: "path", Value: "~/Library/LaunchAgents/com.acme.prod.agent.plist"},
				{Name: "mode", Value: "0644"},
			},
		},
	}
	if len(got) != len(want) {
		t.Fatalf("Verification = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].Name != want[i].Name || got[i].Status != want[i].Status {
			t.Errorf("Verification[%d] = %+v, want %+v", i, got[i], want[i])
		}
		if len(got[i].Facts) != len(want[i].Facts) {
			t.Fatalf("Verification[%d].Facts = %+v, want %+v", i, got[i].Facts, want[i].Facts)
		}
		for j := range want[i].Facts {
			if got[i].Facts[j] != want[i].Facts[j] {
				t.Errorf("Verification[%d].Facts[%d] = %+v, want %+v", i, j, got[i].Facts[j], want[i].Facts[j])
			}
		}
	}
}

// TestToProblemDoc_ProjectsLocationAndRemedies is ZYS-823's guard on the
// wire-format ("evo.run") side of the same projection internal/render's
// toJSONProblems test covers: Location and Remedies must both reach
// ProblemDoc, not just the Code/EvidenceTail fields other tests in this
// file already exercise. Severity/Evidence/Fields are not asserted here:
// ProblemDoc no longer projects them at all, because core.Problem has no
// exported ProblemOption that can ever set them (see ProblemDoc's doc
// comment).
func TestToProblemDoc_ProjectsLocationAndRemedies(t *testing.T) {
	got := ToProblemDoc(core.Problem{
		Summary:  "build failed",
		Location: &core.SourceLocation{Path: "main.go", Line: 12, Column: 3},
		Actions:  []core.Action{{Label: "rerun", Command: &core.CommandSpec{Executable: "go", Args: []string{"build", "./..."}}}},
	})
	if got.Location == nil || got.Location.Path != "main.go" || got.Location.Line != 12 || got.Location.Column != 3 {
		t.Fatalf("Location = %+v, want {main.go 12 3}", got.Location)
	}
	if len(got.Remedies) != 1 || got.Remedies[0].Label != "rerun" {
		t.Fatalf("Remedies = %+v, want a [rerun] action", got.Remedies)
	}
}

// TestEncodeRun_ProblemLocationAndRemediesSurviveEncoding is the end-to-end
// proof: Location and Remedies must both survive a real EncodeRun call, not
// just the ToProblemDoc unit above.
func TestEncodeRun_ProblemLocationAndRemediesSurviveEncoding(t *testing.T) {
	result := core.Result{Conclusion: withConc(func(c *core.Conclusion) {
		c.Tasks = []core.TaskSnapshot{
			core.NewTaskSnapshot(core.TaskSnapshot{
				ID: "task_1", Name: "build", State: core.Done,
				Problems: []core.Problem{{
					Code: "A1", Summary: "finding",
					Location: &core.SourceLocation{Path: "main.go", Line: 12, Column: 3},
					Actions:  []core.Action{{Label: "rerun"}},
				}},
			}, time.Time{}, false, false),
		}
	})}
	encoded, err := EncodeRun(result, testEvoVersion)
	if err != nil {
		t.Fatalf("EncodeRun: %v", err)
	}
	body := string(encoded)
	for _, want := range []string{`"line": 12`, `"rerun"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("encoded evo.run missing %q:\n%s", want, body)
		}
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
