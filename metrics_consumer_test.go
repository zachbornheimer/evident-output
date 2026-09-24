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
	// A caller-supplied Terminal renders the live (TTY) projection instead.
	cfg.Title, cfg.Color, cfg.Plain = "prune", evo.ColorNever, cfg.Terminal == nil
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
		Defined:          4,
		Entered:          3,
		Verified:         1,
		VerifiedCurrent:  1,
		DependencyWait:   ticks(2),
		SchedulerWait:    ticks(3),
		Running:          ticks(3),
		Definition:       ticks(3),
		// fetch waits on the slowest category: one tick, then its own.
		CriticalPath:    ticks(2),
		PeakConcurrency: 1,
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
			Metrics map[string]any `json:"metrics"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(machineDocument(t, run.out)), &doc); err != nil {
		t.Fatal(err)
	}
	wantMetrics := map[string]float64{
		"tasks": 4, "executed": 3, "already_satisfied": 1, "defined": 4, "entered": 3,
		"verified": 1, "verified_current": 1, "dependency_wait_ms": 2000, "scheduler_wait_ms": 3000,
		"running_ms": 3000, "definition_ms": 3000, "critical_path_ms": 2000, "peak_concurrency": 1,
	}
	for key, want := range wantMetrics {
		if got, _ := doc.Data.Metrics[key].(float64); got != want {
			t.Errorf("data.metrics.%s = %v, want %v", key, doc.Data.Metrics[key], want)
		}
	}
	rates, _ := doc.Data.Metrics["rates"].(map[string]any)
	if got, _ := rates["callback_entry"].(float64); got != 0.75 {
		t.Errorf("data.metrics.rates.callback_entry = %v, want 0.75", rates["callback_entry"])
	}
	wantFetch := map[string]int64{
		"queued_ms": 2000, "running_ms": 1000, "total_ms": 3000, "awaiting_definition_ms": 0,
		"dependency_wait_ms": 2000, "scheduler_wait_ms": 0, "definition_ms": 1000,
		"evidence_ms": 0, "provenance_ms": 0, "tracked_state_ms": 0,
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
	assertRunFinishedPayloadConforms(t, finished)
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
	if got := group.Snapshot().PeakConcurrency(); got != 3 {
		t.Fatalf("Group PeakConcurrency() = %d, want 3", got)
	}
	var doc struct {
		Data struct {
			Collections []struct {
				Name            string `json:"name"`
				PeakConcurrency *int   `json:"peak_concurrency"`
			} `json:"collections"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(machineDocument(t, out)), &doc); err != nil {
		t.Fatal(err)
	}
	for _, col := range doc.Data.Collections {
		if col.Name == "categories" && (col.PeakConcurrency == nil || *col.PeakConcurrency != 3) {
			t.Fatalf("collection %q peak_concurrency = %v, want 3", col.Name, col.PeakConcurrency)
		}
	}
}
