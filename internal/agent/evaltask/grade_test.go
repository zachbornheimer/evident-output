package evaltask_test

import (
	"context"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/zachbornheimer/evident-output/internal/agent/evaltask"
	"github.com/zachbornheimer/evident-output/internal/agent/harness"
	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// fakeRunner answers every command from a fixed table and records them.
type fakeRunner struct {
	results map[string]evaltask.CommandResult
	ran     []evaltask.Command
}

func (f *fakeRunner) Run(_ context.Context, cmd evaltask.Command) (evaltask.CommandResult, error) {
	f.ran = append(f.ran, cmd)
	return f.results[cmd.Name], nil
}

const pruneDocument = `{"data":{"collections":[
  {"id":"c1","parent_id":"","name":"prune","kind":"group","state":"done"},
  {"id":"c2","parent_id":"c1","name":"repository","kind":"sequence","state":"done"}],
 "tasks":[{"parent_id":"c2","name":"b","state":"done"},{"parent_id":"c2","name":"a","state":"done"}]}}`

func TestParseTopology_NestsByParentAndSortsSiblings(t *testing.T) {
	got, err := evaltask.ParseTopology([]byte(pruneDocument))
	if err != nil {
		t.Fatalf("ParseTopology: %v", err)
	}
	want := []evaltask.Node{{Name: "prune", Kind: "group", State: "done", Children: []evaltask.Node{
		{Name: "repository", Kind: "sequence", State: "done", Children: []evaltask.Node{
			{Name: "a", Kind: "task", State: "done"}, {Name: "b", Kind: "task", State: "done"}}}}}}
	if diff := evaltask.DiffTopology(got, want); diff != "" {
		t.Fatalf("tree differs:\n%s", diff)
	}
	want[0].Children[0].Children[0].State = "failed"
	if evaltask.DiffTopology(got, want) == "" {
		t.Fatal("a differing state must be reported")
	}
}

func TestRunOrder_ReportsEdgesTheRunBroke(t *testing.T) {
	stream := `{"seq":1,"type":"task.declared","entity_id":"t1","payload":{"name":"first"}}
{"seq":2,"type":"task.declared","entity_id":"t2","payload":{"name":"second"}}
{"seq":3,"type":"task.started","entity_id":"t2"}
{"seq":4,"type":"task.started","entity_id":"t1"}
{"seq":5,"type":"task.finished","entity_id":"t1"}
`
	order, err := evaltask.ParseRunOrder([]byte(stream))
	if err != nil {
		t.Fatalf("ParseRunOrder: %v", err)
	}
	got := order.Violations([]evaltask.OrderEdge{{Before: "first", After: "second"}, {Before: "second", After: "first"}})
	if len(got) != 2 {
		t.Fatalf("violations = %v, want both edges broken (second never finished, second started before first finished)", got)
	}
}

func TestBannedPatterns_FlagsEachSeededBadShape(t *testing.T) {
	cases := map[string]string{
		evaltask.PatternOuterHandoff:     `var n []string; t.Define(func(ctx context.Context) error { n = nil; return nil })`,
		evaltask.PatternGoroutine:        `go work()`,
		evaltask.PatternRedundantAfter:   `s := out.Sequence("s"); a := s.Task("a"); s.Task("b").After(a)`,
		evaltask.PatternGiantLoopTask:    `t.Define(func(ctx context.Context) error { for range xs {}; return nil })`,
		evaltask.PatternPresentationTask: `out.Task("Run summary")`,
		evaltask.PatternDeclareInDefine:  `t.Define(func(ctx context.Context) error { g.Task("x"); return nil })`,
		evaltask.PatternLocalContainer:   `type Container interface{ Up() }`,
	}
	for pattern, body := range cases {
		t.Run(pattern, func(t *testing.T) {
			src := "package main\nimport \"context\"\nvar _ context.Context\nfunc f() {\n" + body + "\n}\n"
			if pattern == evaltask.PatternLocalContainer {
				src = "package main\n" + body + "\n"
			}
			got, err := evaltask.BannedPatterns(fstest.MapFS{"main.go": {Data: []byte(src)}}, []string{pattern})
			if err != nil {
				t.Fatalf("BannedPatterns: %v", err)
			}
			if !slices.Equal(got, []string{pattern}) {
				t.Fatalf("fired %v, want [%s] on:\n%s", got, pattern, src)
			}
		})
	}
}

func TestBannedPatterns_UnknownPatternIsAnError(t *testing.T) {
	_, err := evaltask.BannedPatterns(fstest.MapFS{"main.go": {Data: []byte("package main\n")}}, []string{"typo"})
	if err == nil {
		t.Fatal("an unknown pattern id must not silently pass")
	}
}

// The grader reports a build failure as data and never runs a binary that
// did not build.
func TestGrade_BuildFailureStopsBeforeRun(t *testing.T) {
	runner := &fakeRunner{results: map[string]evaltask.CommandResult{"go": {ExitCode: 1, Stderr: "undefined: x"}}}
	grader := evaltask.Grader{RepoRoot: repoRoot(t), Runner: runner}
	_, tasks, _ := testdata(t)
	task := tasks[0]
	reference, err := task.ReferenceFS()
	if err != nil {
		t.Fatalf("ReferenceFS: %v", err)
	}
	report, err := grader.Grade(context.Background(), task, reference, t.TempDir())
	if err != nil {
		t.Fatalf("Grade: %v", err)
	}
	if report.Compiles || report.BuildOutput != "undefined: x" || report.Passed() {
		t.Fatalf("report = %+v", report)
	}
	if len(runner.ran) != 1 {
		t.Fatalf("ran %d commands, want only the build", len(runner.ran))
	}
}

// Every fixture, including those of blocked tasks, builds on its own.
func TestFixtures_AllBuild(t *testing.T) {
	_, tasks, _ := testdata(t)
	runner := evaltask.ExecRunner{}
	for _, task := range tasks {
		t.Run(task.ID, func(t *testing.T) {
			fixture, err := task.FixtureFS()
			if err != nil {
				t.Fatalf("FixtureFS: %v", err)
			}
			sandbox, err := evaltask.NewSandbox(t.TempDir(), repoRoot(t), fixture, fstest.MapFS{})
			if err != nil {
				t.Fatalf("NewSandbox: %v", err)
			}
			res, err := runner.Run(context.Background(), evaltask.Command{
				Dir: sandbox.Root, Env: []string{"GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off"},
				Name: "go", Args: []string{"build", "./fixture"},
			})
			if err != nil || res.ExitCode != 0 {
				t.Fatalf("fixture does not build: %v\n%s", err, res.Stderr)
			}
		})
	}
}

// the model-backed driver plugs into the repair loop through Fixer.
type scriptedFixer struct{ fixed string }

func (s scriptedFixer) Fix(context.Context, string, []review.Finding) (string, error) {
	return s.fixed, nil
}

func TestFixerSeam_RepairLoopAcceptsAnyFixer(t *testing.T) {
	bad := "package p\nimport evo \"github.com/zachbornheimer/evident-output\"\nfunc f() {\n\tout := evo.Init(evo.Config{})\n\tt := out.Task(\"x\")\n\tt.Start()\n\tt.Define(work)\n}\n"
	good := "package p\nimport evo \"github.com/zachbornheimer/evident-output\"\nfunc f() {\n\tout := evo.Init(evo.Config{})\n\tt := out.Task(\"x\")\n\tt.Define(work)\n}\n"
	loop, err := harness.RunRepairLoopWith(context.Background(), scriptedFixer{fixed: good}, bad, harness.DefaultMaxCycles)
	if err != nil {
		t.Fatalf("RunRepairLoopWith: %v", err)
	}
	if !loop.ReachedClean || loop.Cycles != 2 {
		t.Fatalf("loop = %+v, want clean after the scripted fix", loop)
	}
	stuck, err := harness.RunRepairLoopWith(context.Background(), scriptedFixer{fixed: bad}, bad, harness.DefaultMaxCycles)
	if err != nil || stuck.ReachedClean {
		t.Fatalf("a fixer that changes nothing must stop unclean: %+v %v", stuck, err)
	}
}
