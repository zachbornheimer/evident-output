package evo_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output/v2"
	"github.com/zachbornheimer/evident-output/v2/testkit/wireschema"
)

// publishedRunSchema and publishedEventSchema are the files a consumer downloads.
const (
	publishedRunSchema   = "schema/run.v3.json"
	publishedEventSchema = "schema/event.v2.json"
)

// realOutputScenario is one program whose machine output the published schemas must accept.
type realOutputScenario struct {
	name string
	run  func(t *testing.T, mode evo.Output) contractRun
}

// realOutputScenarios are representative programs: every Outcome, a Problem with every option, a dry run,
// current operations, a same-output conflict, a refused Confirm, a Result payload (text and not UTF-8),
// and a Task cancelled mid-progress.
func realOutputScenarios() []realOutputScenario {
	scenarios := []realOutputScenario{
		{"problem", problemScenario},
		{"dry run", dryRunScenario},
		{"current operations", currentOperationsScenario},
		{"conflict", conflictScenario},
		{"confirm refused", confirmRefusedScenario},
		{"result text", resultScenario("bin/app\n")},
		{"result bytes", resultScenario("\xff\xfe binary")},
		{"cancelled mid progress", cancelledMidProgressScenario},
		{"group and sequence", structureScenario},
	}
	for _, outcome := range outcomeScenarios() {
		scenarios = append(scenarios, realOutputScenario{string(outcome.want), outcome.run})
	}
	return scenarios
}

func problemScenario(t *testing.T, mode evo.Output) contractRun {
	return executeContract(context.Background(), t, evo.Config{Name: "problem", Output: mode},
		func(_ context.Context, r *evo.Run) error {
			task := r.Task("lint")
			task.Define(func(context.Context) error {
				task.Fact("files", "12")
				task.Problem("protected ref", evo.On("origin/main"), evo.Code("protected"),
					evo.Detail("pushes are blocked"), evo.Next(evo.Action{Label: "ask an admin"}))
				return nil
			})
			return nil
		})
}

func dryRunScenario(t *testing.T, mode evo.Output) contractRun {
	return executeContract(context.Background(), t, evo.Config{Name: "dry", Output: mode, DryRun: true},
		func(_ context.Context, r *evo.Run) error {
			r.Task("prune").Define(func(ctx context.Context) error {
				return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "branch", Quantity: 3},
					func(context.Context) error { return nil })
			})
			return nil
		})
}

func currentOperationsScenario(t *testing.T, mode evo.Output) contractRun {
	world := newFreshnessWorld(t)
	world.write(t, "in.txt", "one\n")
	out := world.path("out.txt")
	cfg := evo.Config{Name: "current", Output: mode, State: evo.State{Dir: world.state, AppID: "schema"}}
	body := func(_ context.Context, r *evo.Run) error {
		r.Task("generate").Define(func(ctx context.Context) error {
			return evo.File(ctx, evo.FileSpec{Path: out, Contents: []byte("generated\n")})
		})
		return nil
	}
	executeContract(context.Background(), t, cfg, body) // the first run writes; the second finds it current
	return executeContract(context.Background(), t, cfg, body)
}

func conflictScenario(t *testing.T, mode evo.Output) contractRun {
	return newOverturnWorld(t).conflictingRun(context.Background(), t, mode)
}

func confirmRefusedScenario(t *testing.T, mode evo.Output) contractRun {
	run, _ := confirmFromDefine(t, evo.Config{Name: "confirm", Output: mode})
	return run
}

func resultScenario(payload string) func(*testing.T, evo.Output) contractRun {
	return func(t *testing.T, mode evo.Output) contractRun {
		return executeContract(context.Background(), t, evo.Config{Name: "result", Output: mode},
			func(_ context.Context, r *evo.Run) error {
				r.Task("emit").Define(func(context.Context) error {
					_, err := r.ResultWriter().Write([]byte(payload))
					return err
				})
				return nil
			})
	}
}

func cancelledMidProgressScenario(t *testing.T, mode evo.Output) contractRun {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	go func() {
		<-started
		cancel()
	}()
	return executeContract(ctx, t, evo.Config{Name: "progress", Output: mode},
		func(_ context.Context, r *evo.Run) error {
			task := r.Task("sync")
			task.Define(func(taskCtx context.Context) error {
				task.Progress(3, 10).Doing("syncing widget-3")
				close(started)
				select {
				case <-taskCtx.Done():
					return taskCtx.Err()
				case <-time.After(callbackPatience):
					return errors.New("the run's cancellation never reached the callback")
				}
			})
			return nil
		})
}

func structureScenario(t *testing.T, mode evo.Output) contractRun {
	return executeContract(context.Background(), t, evo.Config{Name: "structure", Output: mode},
		func(_ context.Context, r *evo.Run) error {
			group := r.Group("checks")
			group.Task("fmt").Define(func(context.Context) error { return nil })
			group.Task("vet").Define(func(context.Context) error { return errors.New("vet failed") })
			steps := r.Sequence("release")
			first := steps.Task("tag")
			first.Define(func(context.Context) error { return nil })
			steps.Task("push").After(first).Define(func(context.Context) error { return nil })
			return nil
		})
}

// publishedSchema reads a schema file as shipped, and with wireschema.Strict's undeclared-field overlay.
func publishedSchema(t *testing.T, path string) (shipped, strict []byte) {
	t.Helper()
	shipped, err := os.ReadFile(filepath.FromSlash(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	strict, err = wireschema.Strict(shipped)
	if err != nil {
		t.Fatalf("wireschema.Strict(%s): %v", path, err)
	}
	return shipped, strict
}

// validateAgainstPublished fails t for every way doc breaks the schema as shipped or with no undeclared field.
func validateAgainstPublished(t *testing.T, path string, doc []byte) {
	t.Helper()
	shipped, strict := publishedSchema(t, path)
	for name, schema := range map[string][]byte{"as shipped": shipped, "strict": strict} {
		if err := wireschema.Validate(schema, doc); err != nil {
			t.Errorf("document breaks %s (%s):\n%v\n%s", path, name, err, doc)
		}
	}
}

// realOutput is what one scenario wrote to stdout in one machine mode.
type realOutput struct {
	scenario string
	mode     evo.Output
	stdout   string
}

// realMachineOutputs runs every scenario in JSON and JSONL.
func realMachineOutputs(t *testing.T) []realOutput {
	t.Helper()
	var outputs []realOutput
	for _, scenario := range realOutputScenarios() {
		for _, mode := range []evo.Output{evo.JSON, evo.JSONL} {
			run := scenario.run(t, mode)
			if strings.TrimSpace(run.Stdout) == "" {
				t.Fatalf("%s/%s wrote nothing to stdout:\n%s", scenario.name, outputName[mode], run.Stderr)
			}
			outputs = append(outputs, realOutput{scenario.name, mode, run.Stdout})
		}
	}
	return outputs
}

// TestPublishedSchemasValidateRealOutputStrictly runs representative programs in JSON and JSONL and holds
// every evo.run document and evo.event line to the schema files as published. The validator has no
// null exemption, so a nullable field the schema does not declare fails here as it fails ajv or jsonschema.
func TestPublishedSchemasValidateRealOutputStrictly(t *testing.T) {
	documents, events := 0, 0
	for _, out := range realMachineOutputs(t) {
		dumpRealOutput(t, out)
		switch out.mode {
		case evo.JSON:
			validateAgainstPublished(t, publishedRunSchema, []byte(out.stdout))
			documents++
		case evo.JSONL:
			for line := range strings.SplitSeq(strings.TrimSpace(out.stdout), "\n") {
				validateAgainstPublished(t, publishedEventSchema, []byte(line))
				events++
			}
		}
	}
	if documents == 0 || events == 0 {
		t.Fatalf("validated %d documents and %d events; a vacuous pass proves nothing", documents, events)
	}
}

// realOutputDirEnv names a directory the test writes each captured output to, for tools/crosscheck-schemas.py.
const realOutputDirEnv = "EVO_REAL_OUTPUT_DIR"

// dumpRealOutput writes out where tools/crosscheck-schemas.py reads it, when realOutputDirEnv is set.
func dumpRealOutput(t *testing.T, out realOutput) {
	t.Helper()
	dir, ok := os.LookupEnv(realOutputDirEnv)
	if !ok || dir == "" {
		return
	}
	ext := map[evo.Output]string{evo.JSON: ".json", evo.JSONL: ".jsonl"}[out.mode]
	name := strings.ReplaceAll(out.scenario, " ", "-") + ext
	if err := os.WriteFile(filepath.Join(dir, name), []byte(out.stdout), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
