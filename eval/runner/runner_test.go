package runner_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/eval/driver"
	"github.com/zachbornheimer/evident-output/eval/runner"
	"github.com/zachbornheimer/evident-output/internal/agent/evaltask"
)

const (
	taskPrune = "T1-zq-prune"
	trapX1    = "X1-outer-variable-handoff"
	// Placeholder only: no network client exists in these tests.
	testCredential = "not-a-real-credential"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

func testdata(t *testing.T) fs.FS {
	t.Helper()
	return os.DirFS(filepath.Join(repoRoot(t), "internal", "agent", "evaltask", "testdata"))
}

func loadTask(t *testing.T, id string) evaltask.Task {
	t.Helper()
	tasks, err := evaltask.LoadTasks(testdata(t))
	if err != nil {
		t.Fatalf("LoadTasks: %v", err)
	}
	for _, task := range tasks {
		if task.ID == id {
			return task
		}
	}
	t.Fatalf("task %s not found", id)
	return evaltask.Task{}
}

func filesOf(t *testing.T, tree fs.FS) map[string]string {
	t.Helper()
	files := map[string]string{}
	entries, err := fs.ReadDir(tree, ".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		data, err := fs.ReadFile(tree, entry.Name())
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		files[entry.Name()] = string(data)
	}
	return files
}

func referenceFiles(t *testing.T, task evaltask.Task) map[string]string {
	t.Helper()
	tree, err := task.ReferenceFS()
	if err != nil {
		t.Fatalf("ReferenceFS: %v", err)
	}
	return filesOf(t, tree)
}

func trapFiles(t *testing.T, id string) map[string]string {
	t.Helper()
	traps, err := evaltask.LoadTraps(testdata(t))
	if err != nil {
		t.Fatalf("LoadTraps: %v", err)
	}
	for _, trap := range traps {
		if strings.HasPrefix(trap.ID, strings.SplitN(id, "-", 2)[0]) {
			tree, err := trap.AnswerFS()
			if err != nil {
				t.Fatalf("AnswerFS: %v", err)
			}
			return filesOf(t, tree)
		}
	}
	t.Fatalf("trap %s not found", id)
	return nil
}

type fakeServer struct{}

func (fakeServer) Tools(context.Context) ([]driver.ToolDef, error) {
	var defs []driver.ToolDef
	for _, name := range []string{
		"evident_output_list_sections", "evident_output_get_documentation", "evident_output_review",
		"evident_output_explain", "evident_output_preview",
	} {
		defs = append(defs, driver.ToolDef{Name: name, InputSchema: json.RawMessage(`{"type":"object"}`)})
	}
	return defs, nil
}

func (fakeServer) Call(_ context.Context, name string, _ json.RawMessage) (driver.ToolResult, error) {
	if name == "evident_output_list_sections" {
		return driver.ToolResult{Structured: json.RawMessage(`{"sections":[{"id":"a"}]}`)}, nil
	}
	return driver.ToolResult{Structured: json.RawMessage(`{"sections":[{"id":"a","title":"A","body":"docs"}]}`)}, nil
}

func (fakeServer) Close() error { return nil }

type memoryCache struct{ data map[string][]byte }

func (m *memoryCache) Get(key string) ([]byte, bool, error) { v, ok := m.data[key]; return v, ok, nil }
func (m *memoryCache) Put(key string, v []byte) error {
	if m.data == nil {
		m.data = map[string][]byte{}
	}
	m.data[key] = v
	return nil
}

type collectSink struct{ records []runner.Record }

func (c *collectSink) Write(r runner.Record) error { c.records = append(c.records, r); return nil }

func verifiedPrices() driver.PriceTable {
	return driver.PriceTable{driver.ModelInnerLoop: {InputPerMTok: 1_000_000, OutputPerMTok: 1_000_000, Verified: true}}
}

func baseConfig(t *testing.T) runner.Config {
	t.Helper()
	return runner.Config{
		TaskIDs: []string{taskPrune}, Models: []string{driver.ModelInnerLoop}, Samples: 1, MaxUSD: 1000,
		ConfirmSpend: true, Credential: testCredential, Prices: verifiedPrices(), RepoRoot: repoRoot(t),
	}
}

func scriptedRunner(t *testing.T, model driver.Model, sink *collectSink) runner.Runner {
	t.Helper()
	return runner.Runner{
		Grader:   evaltask.Grader{RepoRoot: repoRoot(t), Runner: evaltask.ExecRunner{}},
		Server:   fakeServer{},
		Cache:    &memoryCache{},
		NewModel: func(string) driver.Model { return model },
		WorkDir:  driver.TempWorkDir,
		SinkFor:  func(string) (runner.Sink, error) { return sink, nil },
	}
}

func submitTurn(files map[string]string, usage driver.Usage) []driver.ScriptedTurn {
	return []driver.ScriptedTurn{driver.ToolCallTurn("s", "submit", map[string]any{"files": files}, usage)}
}

// The reference answer, submitted by a scripted model, is graded as a pass.
func TestRun_ScriptedReferenceAnswerPasses(t *testing.T) {
	task := loadTask(t, taskPrune)
	model := &driver.ScriptedModel{Turns: submitTurn(referenceFiles(t, task), driver.Usage{InputTokens: 10, OutputTokens: 5})}
	sink := &collectSink{}
	summary, err := scriptedRunner(t, model, sink).Run(context.Background(), baseConfig(t), []evaltask.Task{task})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Passed != 1 || len(sink.records) != 1 {
		t.Fatalf("summary = %+v, records = %d", summary, len(sink.records))
	}
	record := sink.records[0]
	if !record.Passed || record.Grade == nil || !record.Grade.Passed() {
		t.Fatalf("reference must pass: %+v", record.Grade)
	}
	if record.Tokens.InputTokens != 10 || record.CostUSD != 15 {
		t.Errorf("accounting: tokens=%+v cost=%v, want 10 in / 5 out = $15", record.Tokens, record.CostUSD)
	}
}

// The X1 trap, submitted the same way, is graded as a failure.
func TestRun_ScriptedTrapAnswerFails(t *testing.T) {
	task := loadTask(t, taskPrune)
	model := &driver.ScriptedModel{Turns: submitTurn(trapFiles(t, trapX1), driver.Usage{})}
	sink := &collectSink{}
	if _, err := scriptedRunner(t, model, sink).Run(context.Background(), baseConfig(t), []evaltask.Task{task}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	record := sink.records[0]
	if record.Passed || record.Grade == nil {
		t.Fatalf("trap must fail grading: %+v", record)
	}
	if len(record.Grade.BannedPatterns) == 0 && record.Grade.ReviewClean {
		t.Errorf("trap neither tripped a banned pattern nor review: %+v", record.Grade)
	}
}

func TestRun_AbortsWholeRunAtSpendCap(t *testing.T) {
	task := loadTask(t, taskPrune)
	var turns []driver.ScriptedTurn
	for range 3 {
		turns = append(turns, submitTurn(referenceFiles(t, task), driver.Usage{InputTokens: 600})...)
	}
	cfg := baseConfig(t)
	cfg.Samples = 3
	cfg.MaxUSD = 1000 // two $600 turns pass the cap on the second
	sink := &collectSink{}
	summary, err := scriptedRunner(t, &driver.ScriptedModel{Turns: turns}, sink).Run(context.Background(), cfg, []evaltask.Task{task})
	if !errors.Is(err, driver.ErrSpendCapReached) || !summary.Aborted {
		t.Fatalf("err = %v, summary = %+v", err, summary)
	}
	if len(sink.records) != 2 {
		t.Errorf("want the run to stop after 2 of 3 samples, wrote %d records", len(sink.records))
	}
	if sink.records[1].Error == "" {
		t.Error("the sample that tripped the cap must record why")
	}
}

func TestConfig_RefusesToStartWithoutGuards(t *testing.T) {
	ready := baseConfig(t)
	cases := map[string]func(*runner.Config){
		"no credential":     func(c *runner.Config) { c.Credential = "" },
		"no max-usd":        func(c *runner.Config) { c.MaxUSD = 0 },
		"no confirm-spend":  func(c *runner.Config) { c.ConfirmSpend = false },
		"no tasks selected": func(c *runner.Config) { c.TaskIDs = nil },
	}
	if err := ready.Validate(); err != nil {
		t.Fatalf("ready config rejected: %v", err)
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := ready
			mutate(&cfg)
			if err := cfg.Validate(); !errors.Is(err, runner.ErrRefusedToStart) {
				t.Fatalf("Validate = %v, want ErrRefusedToStart", err)
			}
		})
	}
}

func TestRun_RefusedConfigNeverTouchesTheModel(t *testing.T) {
	model := &driver.ScriptedModel{}
	cfg := baseConfig(t)
	cfg.ConfirmSpend = false
	_, err := scriptedRunner(t, model, &collectSink{}).Run(context.Background(), cfg, []evaltask.Task{loadTask(t, taskPrune)})
	if !errors.Is(err, runner.ErrRefusedToStart) || len(model.Requests) != 0 {
		t.Fatalf("err = %v, requests = %d", err, len(model.Requests))
	}
}

func TestConfig_DryRunNeedsNoGuards(t *testing.T) {
	cfg := runner.Config{AllReady: true, Models: []string{driver.ModelInnerLoop}, Samples: 3, DryRun: true}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("dry run rejected: %v", err)
	}
}

func TestBuildPlan_RefusesUnverifiedPriceAndBoundsWorstCase(t *testing.T) {
	task := loadTask(t, taskPrune)
	cfg := baseConfig(t)
	cfg.Prices = driver.PriceTable(driver.PlaceholderPrices)
	if _, err := runner.BuildPlan(cfg, []evaltask.Task{task}); !errors.Is(err, runner.ErrRefusedToStart) {
		t.Fatalf("unverified price: err = %v", err)
	}
	cfg.Prices = verifiedPrices()
	plan, err := runner.BuildPlan(cfg, []evaltask.Task{task})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	perTurn := float64(runner.WorstCaseContextTokens + driver.DefaultMaxTokens)
	if want := perTurn * driver.MaxToolTurns; plan.WorstCaseUSD != want {
		t.Errorf("worst case = %v, want %v", plan.WorstCaseUSD, want)
	}
	var out bytes.Buffer
	plan.Print(&out, cfg.MaxUSD)
	if !strings.Contains(out.String(), taskPrune) {
		t.Errorf("plan output omits the task: %s", out.String())
	}
}

func TestSelectTasks_AllReadySkipsBlockedAndNamedBlockedIsAnError(t *testing.T) {
	all, err := evaltask.LoadTasks(testdata(t))
	if err != nil {
		t.Fatalf("LoadTasks: %v", err)
	}
	ready, err := runner.SelectTasks(all, nil, true)
	if err != nil {
		t.Fatalf("SelectTasks: %v", err)
	}
	for _, task := range ready {
		if task.Blocked() {
			t.Errorf("--all-ready picked blocked task %s", task.ID)
		}
	}
	for _, task := range all {
		if task.Blocked() {
			if _, err := runner.SelectTasks(all, []string{task.ID}, false); err == nil {
				t.Errorf("naming blocked task %s must fail", task.ID)
			}
			break
		}
	}
	if _, err := runner.SelectTasks(all, []string{"nope"}, false); err == nil {
		t.Error("an unknown id must fail")
	}
}

func TestJSONLSink_WritesOneLinePerRecord(t *testing.T) {
	var out bytes.Buffer
	sink := runner.JSONLSink{Out: &out}
	for i := 1; i <= 2; i++ {
		if err := sink.Write(runner.Record{Task: taskPrune, Sample: i, Tokens: driver.Usage{CacheReadTokens: 7}}); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"cache_read":7`) {
		t.Errorf("transcript = %q", out.String())
	}
}
