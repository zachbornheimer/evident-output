package evo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// The §39 consumer fixture: zq prune's shape (contract §18) — three
// independent categories in a Group, then a fetch that waits for all of
// them — run with a one-slot scheduler on a fake clock so every wait is a
// known number of ticks. The caller writes no timing code; Evo derives the
// per-Task lifecycle truth and the run aggregate and projects both.

const pruneTick = time.Second

type pruneMetricsRun struct {
	out      *evo.Output
	rendered string
	tasks    map[string]evo.TaskSnapshot
}

// runPruneMetricsFixture declares the whole workflow before any work may
// advance the clock (release gates every callback), so declaration,
// eligibility, start, and settlement times are deterministic.
func runPruneMetricsFixture(t *testing.T, cfg evo.Config) pruneMetricsRun {
	t.Helper()
	clock := testkit.NewClock()
	var buf bytes.Buffer
	cfg.Isolated, cfg.Stdout, cfg.Stderr = true, &buf, io.Discard
	cfg.Title, cfg.Color, cfg.Plain = "prune", evo.ColorNever, true
	cfg.Clock, cfg.MaxConcurrency = clock, 1
	out := evo.Init(cfg)
	t.Cleanup(func() { _ = out.Close() })

	release := make(chan struct{})
	work := func(context.Context) error {
		<-release
		clock.Advance(pruneTick)
		return nil
	}
	categories := out.Group("categories")
	branches := categories.Task("branches").Define(work)
	worktrees := categories.Task("worktrees").Define(work)
	remote := categories.Task("remote-tracking")
	remote.Verify(func(context.Context) (bool, error) { return true, nil })
	remote.Define(work)
	fetch := out.Task("fetch").After(categories).Define(work)
	close(release)
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	tasks := map[string]evo.TaskSnapshot{}
	for _, h := range []*evo.TaskHandle{branches, worktrees, remote, fetch} {
		snap := h.Snapshot()
		tasks[snap.Name] = snap
	}
	return pruneMetricsRun{out: out, rendered: buf.String(), tasks: tasks}
}

func ticks(n int) time.Duration { return time.Duration(n) * pruneTick }

func TestMetrics_TaskTimingSeparatesDependencyWaitSchedulerWaitAndRunning(t *testing.T) {
	t.Parallel()
	run := runPruneMetricsFixture(t, evo.Config{})
	want := map[string][3]time.Duration{ // dependency wait, scheduler wait, running
		"branches":        {0, 0, ticks(1)},
		"worktrees":       {0, ticks(1), ticks(1)},
		"remote-tracking": {0, ticks(2), 0},
		"fetch":           {ticks(2), 0, ticks(1)},
	}
	for name, w := range want {
		timing := run.tasks[name].Timing
		got := [3]time.Duration{timing.DependencyWait(), timing.SchedulerWait(), timing.Running()}
		if got != w {
			t.Errorf("%s: (dependency, scheduler, running) = %v, want %v (timing %+v)", name, got, w, timing)
		}
	}
}

func TestMetrics_ConclusionDerivesTheRunAggregate(t *testing.T) {
	t.Parallel()
	run := runPruneMetricsFixture(t, evo.Config{})
	want := evo.RunMetrics{
		Tasks:            4,
		Executed:         3,
		AlreadySatisfied: 1,
		DependencyWait:   ticks(2),
		SchedulerWait:    ticks(3),
		Running:          ticks(3),
		PeakConcurrency:  1,
	}
	if got := run.out.Conclusion().Metrics(); got != want {
		t.Fatalf("Conclusion().Metrics() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestMetrics_FinalJSONCarriesTaskTimingAndRunMetrics(t *testing.T) {
	t.Parallel()
	run := runPruneMetricsFixture(t, evo.Config{})
	var doc struct {
		Data struct {
			Tasks []struct {
				Name   string           `json:"name"`
				Timing map[string]int64 `json:"timing"`
			} `json:"tasks"`
			Metrics map[string]int64 `json:"metrics"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(machineDocument(t, run.out)), &doc); err != nil {
		t.Fatal(err)
	}
	wantMetrics := map[string]int64{
		"tasks": 4, "executed": 3, "already_satisfied": 1, "no_work": 0,
		"dependency_wait_ms": 2000, "scheduler_wait_ms": 3000, "running_ms": 3000, "peak_concurrency": 1,
	}
	if !mapsEqual(doc.Data.Metrics, wantMetrics) {
		t.Fatalf("data.metrics = %v, want %v", doc.Data.Metrics, wantMetrics)
	}
	wantFetch := map[string]int64{
		"queued_ms": 2000, "running_ms": 1000, "total_ms": 3000,
		"dependency_wait_ms": 2000, "scheduler_wait_ms": 0,
	}
	for _, task := range doc.Data.Tasks {
		if task.Name == "fetch" && !mapsEqual(task.Timing, wantFetch) {
			t.Fatalf("fetch timing = %v, want %v", task.Timing, wantFetch)
		}
	}
}

func mapsEqual(got, want map[string]int64) bool {
	if len(got) != len(want) {
		return false
	}
	for k, v := range want {
		if g, ok := got[k]; !ok || g != v {
			return false
		}
	}
	return true
}

func TestMetrics_JSONLStreamsEligibilityWhenItHappensAndMetricsAtRunFinished(t *testing.T) {
	t.Parallel()
	run := runPruneMetricsFixture(t, evo.Config{Format: evo.FormatJSONL})
	var order []string
	var finished map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(run.rendered), "\n") {
		var ev struct {
			Type     string         `json:"type"`
			EntityID string         `json:"entity_id"`
			Payload  map[string]any `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("decode %q: %v", line, err)
		}
		switch ev.Type {
		case "task.eligible", "task.started":
			order = append(order, ev.Type+" "+run.nameOf(ev.EntityID))
		case "run.finished":
			finished = ev.Payload
		}
	}
	// worktrees became eligible at declaration; the one-slot scheduler, not
	// a dependency, is what held it — so eligibility precedes branches'
	// settlement instead of coinciding with worktrees' own start.
	if i, j := indexOf(order, "task.eligible worktrees"), indexOf(order, "task.started worktrees"); i < 0 || j < 0 || i > j || j-i < 2 {
		t.Fatalf("worktrees must be eligible well before it starts; order = %v", order)
	}
	metrics, _ := finished["metrics"].(map[string]any)
	if got, _ := metrics["scheduler_wait_ms"].(float64); got != 3000 {
		t.Fatalf("run.finished metrics = %v, want scheduler_wait_ms 3000", finished)
	}
}

func (r pruneMetricsRun) nameOf(id string) string {
	for name, snap := range r.tasks {
		if snap.ID == id {
			return name
		}
	}
	return id
}

func indexOf(xs []string, want string) int {
	for i, x := range xs {
		if x == want {
			return i
		}
	}
	return -1
}

func TestMetrics_HumanProjectionShowsTimingOnlyUnderVerbose(t *testing.T) {
	t.Parallel()
	const want = "timing  3 executed · 1 already satisfied · 3s running · 2s waiting on dependencies · 3s waiting on capacity · peak 1 concurrent\n"
	verbose := runPruneMetricsFixture(t, evo.Config{Verbosity: evo.VerbosityVerbose})
	if !strings.Contains(verbose.rendered, want) {
		t.Fatalf("verbose output lacks the timing line %q:\n%s", want, verbose.rendered)
	}
	normal := runPruneMetricsFixture(t, evo.Config{})
	if strings.Contains(normal.rendered, "timing  ") {
		t.Fatalf("rows are scarce: normal output must not render timing:\n%s", normal.rendered)
	}
}

func TestMetrics_PeakConcurrencyReflectsOverlappingGroupSiblings(t *testing.T) {
	t.Parallel()
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Stderr: io.Discard, Plain: true, Clock: clock, MaxConcurrency: 3})
	t.Cleanup(func() { _ = out.Close() })
	var started sync.WaitGroup
	started.Add(3)
	release := make(chan struct{})
	group := out.Group("categories")
	for _, name := range []string{"branches", "worktrees", "remote-tracking"} {
		group.Task(name).Define(func(context.Context) error {
			started.Done()
			<-release
			return nil
		})
	}
	started.Wait()
	clock.Advance(pruneTick)
	close(release)
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	m := out.Conclusion().Metrics()
	if m.PeakConcurrency != 3 || m.Running != ticks(3) {
		t.Fatalf("Metrics() = %+v, want peak 3 and 3s running", m)
	}
}
