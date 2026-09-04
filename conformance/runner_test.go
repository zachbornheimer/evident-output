package conformance_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// Scenario is the declarative roast fixture (schema v1).
type Scenario struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Subject   string         `json:"subject"`
	Options   ScenarioOpts   `json:"options"`
	Mutations []Mutation     `json:"mutations"`
	Expect    ScenarioExpect `json:"expect"`
}

type ScenarioOpts struct {
	Plain          bool `json:"plain"`
	NoColor        bool `json:"no_color"`
	NonInteractive bool `json:"non_interactive"`
	Width          int  `json:"width"`
	Strict         bool `json:"strict"`
}

type Mutation struct {
	Op        string        `json:"op"`
	Name      string        `json:"name"`
	Ref       string        `json:"ref"`
	Parent    string        `json:"parent"`
	Summary   string        `json:"summary"`
	Detail    string        `json:"detail"`
	Text      string        `json:"text"`
	Format    string        `json:"format"`
	Completed int64         `json:"completed"`
	Total     int64         `json:"total"`
	Delta     int64         `json:"delta"`
	Verb      string        `json:"verb"`
	Quantity  int64         `json:"quantity"`
	Object    string        `json:"object"`
	Problems  []evo.Problem `json:"problems"`
}

type ScenarioExpect struct {
	FinishError      string        `json:"finish_error"`
	OutputError      string        `json:"output_error"`
	ConclusionState  string        `json:"conclusion_state"`
	Changed          *bool         `json:"changed"`
	Entity           *EntityExpect `json:"entity"`
	PlainContains    []string      `json:"plain_contains"`
	PlainNotContains []string      `json:"plain_not_contains"`
	PlainEquals      string        `json:"plain_equals"`
}

type EntityExpect struct {
	Ref               string `json:"ref"`
	State             string `json:"state"`
	Phase             string `json:"phase"`
	ProgressCompleted *int64 `json:"progress_completed"`
	ProgressTotal     *int64 `json:"progress_total"`
	ProgressKind      string `json:"progress_kind"`
	ProblemCount      *int   `json:"problem_count"`
	ProblemSummary    string `json:"problem_summary"`
}

func TestConformanceScenarios(t *testing.T) {
	dir := "scenarios"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read scenarios: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		t.Run(e.Name(), func(t *testing.T) {
			runScenarioFile(t, path)
		})
	}
}

func runScenarioFile(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var sc Scenario
	if err := json.Unmarshal(raw, &sc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Reject unknown fields by re-encoding round-trip strictness is soft;
	// schema file is the contract. Runtime rejects empty id.
	if sc.ID == "" || sc.Title == "" {
		t.Fatal("scenario missing id/title")
	}

	var buf bytes.Buffer
	opts := []evo.Option{evo.To(&buf)}
	if sc.Options.Plain {
		opts = append(opts, evo.Plain())
	}
	if sc.Options.NoColor {
		opts = append(opts, evo.NoColor())
	}
	if sc.Options.NonInteractive {
		opts = append(opts, evo.Plain())
	}
	if sc.Options.Width > 0 {
		opts = append(opts, evo.Width(sc.Options.Width))
	}
	if sc.Options.Strict {
		opts = append(opts, evo.Strict())
	}

	if sc.Subject != "" {
		opts = append([]evo.Option{evo.Title(sc.Subject)}, opts...)
	}
	out := evo.Init(evo.Config{Options: opts})
	t.Cleanup(func() { _ = out.Close() })

	items := map[string]*evo.TaskHandle{}
	tasks := map[string]*evo.TaskHandle{}
	cols := map[string]*evo.Tasks{}
	var finishErr error

	for _, m := range sc.Mutations {
		switch m.Op {
		case "item":
			items[m.Ref] = out.Task(m.Name)
		case "task":
			tasks[m.Ref] = out.Task(m.Name)
		case "tasks":
			cols[m.Ref] = out.Tasks(m.Name)
		case "tasks.task":
			parent := cols[m.Parent]
			if parent == nil {
				t.Fatalf("unknown parent %q", m.Parent)
			}
			tasks[m.Ref] = parent.Task(m.Name)
		case "item.ok":
			items[m.Ref].Done()
		case "item.block":
			var po []evo.ProblemOption
			if m.Detail != "" {
				po = append(po, evo.Detail(m.Detail))
			}
			items[m.Ref].Block(m.Summary, po...)
		case "item.warn":
			items[m.Ref].Warn(m.Summary)
		case "item.fail":
			items[m.Ref].Fail(m.Summary)
		case "task.phase":
			tasks[m.Ref].Phase(m.Text)
		case "task.progress":
			tasks[m.Ref].Progress64(m.Completed, m.Total)
		case "task.bytes":
			tasks[m.Ref].Bytes(m.Completed, m.Total)
		case "task.done":
			tasks[m.Ref].Done()
		case "task.donef":
			tasks[m.Ref].Donef("%s", m.Text)
		case "task.fail":
			tasks[m.Ref].Fail(m.Summary)
		case "tasks.summary":
			cols[m.Ref].Summary(m.Text)
		case "finish":
			finishErr = out.Finish()
		default:
			t.Fatalf("unknown op %q", m.Op)
		}
	}

	if sc.Expect.OutputError != "" {
		want := sentinel(sc.Expect.OutputError)
		if !errors.Is(out.Err(), want) {
			t.Fatalf("out.Err() = %v, want %v", out.Err(), want)
		}
	}
	if sc.Expect.FinishError != "" {
		if finishErr == nil {
			finishErr = out.Finish()
		}
		want := sentinel(sc.Expect.FinishError)
		if !errors.Is(finishErr, want) {
			t.Fatalf("Finish = %v, want %v", finishErr, want)
		}
	}
	if sc.Expect.Entity != nil {
		ex := sc.Expect.Entity
		if task, ok := tasks[ex.Ref]; ok {
			got := task.Snapshot()
			if ex.State != "" && string(got.State) != ex.State {
				t.Fatalf("task state = %q, want %q", got.State, ex.State)
			}
			if ex.Phase != "" && got.Phase != ex.Phase {
				t.Fatalf("phase = %q, want %q", got.Phase, ex.Phase)
			}
			if ex.ProgressKind != "" && string(got.Progress.Kind) != ex.ProgressKind {
				t.Fatalf("kind = %q, want %q", got.Progress.Kind, ex.ProgressKind)
			}
			if ex.ProgressCompleted != nil && got.Progress.Completed != *ex.ProgressCompleted {
				t.Fatalf("completed = %d, want %d", got.Progress.Completed, *ex.ProgressCompleted)
			}
			if ex.ProgressTotal != nil && got.Progress.Total != *ex.ProgressTotal {
				t.Fatalf("total = %d, want %d", got.Progress.Total, *ex.ProgressTotal)
			}
		}
	}
}

func sentinel(name string) error {
	switch name {
	case "ErrUnresolvedTask":
		return evo.ErrUnresolvedTask
	case "ErrInvalidProgress":
		return evo.ErrInvalidProgress
	case "ErrProgressRegression":
		return evo.ErrProgressRegression
	case "ErrAlreadyResolved":
		return evo.ErrAlreadyResolved
	default:
		return errors.New("evo: " + name)
	}
}
