package wire

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/record"
	"github.com/zachbornheimer/evident-output/internal/wireschema"
)

const testEvoVersion = "1.0.0"

var (
	testStart  = time.Date(2026, 9, 15, 20, 0, 0, 0, time.UTC)
	testFinish = time.Date(2026, 9, 15, 20, 0, 2, 147000000, time.UTC)
)

func baseConclusion() record.Conclusion {
	return record.Conclusion{
		RunID:      "out_1",
		StartedAt:  testStart,
		FinishedAt: testFinish,
	}
}

// runFixture returns the eight §35-required scenarios: success,
// already-satisfied, failure, blocked, cancelled, dry-run, provenance
// revalidation placeholder, partial effects.
func runFixtures() map[string]record.Result {
	return map[string]record.Result{
		"success": {Conclusion: withConc(func(c *record.Conclusion) {
			c.State = record.StateChanged
			c.Changed = true
			c.ExitCode = record.ExitOK
			c.Tasks = []record.TaskSnapshot{
				record.NewTaskSnapshot(record.TaskSnapshot{
					ID: "task_1", Key: "build", Name: "build", State: record.Done,
					Resolution: record.ResolutionExecuted,
				}, time.Time{}, false),
			}
		})},
		"already_satisfied": {Conclusion: withConc(func(c *record.Conclusion) {
			c.State = record.StateReady
			c.ExitCode = record.ExitOK
			c.Tasks = []record.TaskSnapshot{
				record.NewTaskSnapshot(record.TaskSnapshot{
					ID: "task_1", Key: "install", Name: "install", State: record.Done,
					Resolution: record.ResolutionAlreadySatisfied,
					Evidence: record.TaskEvidence{
						Before: record.EvidencePhase{Evaluated: true, Satisfied: true, Source: "verify"},
					},
				}, time.Time{}, false),
			}
		})},
		"failure": {Conclusion: withConc(func(c *record.Conclusion) {
			c.State = record.StateFailed
			c.ExitCode = record.ExitFailed
			c.Tasks = []record.TaskSnapshot{
				record.NewTaskSnapshot(record.TaskSnapshot{
					ID: "task_1", Key: "deploy", Name: "deploy", State: record.Failed,
					Problems: []record.Problem{{Code: "deploy.write", Summary: "failed to write"}},
				}, time.Time{}, false),
			}
		})},
		"blocked": {Conclusion: withConc(func(c *record.Conclusion) {
			c.State = record.StateBlocked
			c.ExitCode = record.ExitBlocked
			c.Tasks = []record.TaskSnapshot{
				record.NewTaskSnapshot(record.TaskSnapshot{
					ID: "task_1", Key: "confirm", Name: "confirm", State: record.Blocked,
				}, time.Time{}, false),
			}
		})},
		"cancelled": {Conclusion: withConc(func(c *record.Conclusion) {
			c.State = record.StateCancelled
			c.Cancelled = true
			c.ExitCode = record.ExitCancelled
			c.Tasks = []record.TaskSnapshot{
				record.NewTaskSnapshot(record.TaskSnapshot{
					ID: "task_1", Key: "scan", Name: "scan", State: record.Done,
				}, time.Time{}, false),
				record.NewTaskSnapshot(record.TaskSnapshot{
					ID: "task_2", Key: "venv", Name: "venv", State: record.Cancelled,
				}, time.Time{}, false),
			}
		})},
		"dry_run": {Conclusion: withConc(func(c *record.Conclusion) {
			c.State = record.StatePlanned
			c.DryRun = true
			c.ExitCode = record.ExitOK
			c.Plans = []record.PlanSnapshot{
				{ID: "plan_1", Subject: "repo", Records: []record.EffectRecord{
					{Verb: "delete", Object: "stale tag", HasQty: true, Quantity: 3},
				}},
			}
		})},
		"provenance_revalidation": {Conclusion: withConc(func(c *record.Conclusion) {
			c.State = record.StateChanged
			c.Changed = true
			c.ExitCode = record.ExitOK
			c.Tasks = []record.TaskSnapshot{
				record.NewTaskSnapshot(record.TaskSnapshot{
					ID: "task_1", Key: "producer", Name: "producer", State: record.Done,
					Resolution: record.ResolutionExecuted,
					Evidence: record.TaskEvidence{
						Before: record.EvidencePhase{Evaluated: false},
						After:  record.EvidencePhase{Evaluated: true, Satisfied: true, Source: "operations"},
					},
				}, time.Time{}, false),
			}
		})},
		"partial_effects": {Conclusion: withConc(func(c *record.Conclusion) {
			c.State = record.StateChanged
			c.Changed = true
			c.Partial = true
			c.ExitCode = record.ExitOK
			c.Changes = []record.ChangesSnapshot{
				{ID: "ch_1", Subject: "repo", Records: []record.EffectRecord{
					{Verb: "deleted", Object: "branch", HasQty: true, Quantity: 1},
				}},
			}
			c.Plans = []record.PlanSnapshot{
				{ID: "plan_1", Subject: "repo", Records: []record.EffectRecord{
					{Verb: "delete", Object: "tag", HasQty: true, Quantity: 2},
				}},
			}
		})},
	}
}

func withConc(mutate func(*record.Conclusion)) record.Conclusion {
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
	schema, err := os.ReadFile("../../../schema/run.v2.json")
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
	result := record.Result{Conclusion: withConc(func(c *record.Conclusion) {
		c.Tasks = []record.TaskSnapshot{
			record.NewTaskSnapshot(record.TaskSnapshot{
				ID: "task_1", Name: "write launch agent", State: record.Failed,
				Verification: []record.VerificationDetail{
					{Name: "contents", Status: record.VerificationSatisfied},
					{
						Name: "permissions", Status: record.VerificationError,
						Facts: []record.Fact{
							{Name: "error", Value: "operation not permitted"},
							{Name: "path", Value: "~/Library/LaunchAgents/com.acme.prod.agent.plist"},
							{Name: "mode", Value: "0644"},
						},
					},
				},
			}, time.Time{}, false),
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

// TestToProblemDoc_ProjectsLocationAndRemedies proves Location and
// Remedies reach ProblemDoc; evo.run is the only machine document that
// carries them (output.v1 is frozen at 1.1).
func TestToProblemDoc_ProjectsLocationAndRemedies(t *testing.T) {
	got := ToProblemDoc(record.Problem{
		Summary:  "build failed",
		Location: &record.SourceLocation{Path: "main.go", Line: 12, Column: 3},
		Actions:  []record.Action{{Label: "rerun", Command: &record.CommandSpec{Executable: "go", Args: []string{"build", "./..."}}}},
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
	result := record.Result{Conclusion: withConc(func(c *record.Conclusion) {
		c.Tasks = []record.TaskSnapshot{
			record.NewTaskSnapshot(record.TaskSnapshot{
				ID: "task_1", Name: "build", State: record.Done,
				Problems: []record.Problem{{
					Code: "A1", Summary: "finding",
					Location: &record.SourceLocation{Path: "main.go", Line: 12, Column: 3},
					Actions:  []record.Action{{Label: "rerun"}},
				}},
			}, time.Time{}, false),
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
		state record.ConclusionState
		want  string
	}{
		{record.StateReady, OutcomeOK},
		{record.StateChanged, OutcomeOK},
		{record.StateWarning, OutcomeOK},
		{record.StatePlanned, OutcomeOK},
		{record.StateBlocked, OutcomeBlocked},
		{record.StateFailed, OutcomeFailed},
		{record.StateCancelled, OutcomeCancelled},
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
	result := record.Result{Conclusion: withConc(func(c *record.Conclusion) {
		c.Tasks = []record.TaskSnapshot{
			record.NewTaskSnapshot(record.TaskSnapshot{
				ID: "task_1", Name: "install", State: record.Done,
				Resolution: record.ResolutionAlreadySatisfied,
				Evidence: record.TaskEvidence{
					Before: record.EvidencePhase{Evaluated: true, Satisfied: true, Source: "verify"},
				},
			}, time.Time{}, false),
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
