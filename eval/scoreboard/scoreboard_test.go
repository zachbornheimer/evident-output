package scoreboard_test

import (
	"bytes"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/eval/runner"
	"github.com/zachbornheimer/evident-output/eval/scoreboard"
)

func rec(task string, passed bool, cycles int, cost float64) runner.Record {
	return runner.Record{Task: task, Model: "m", Passed: passed, CyclesToClean: cycles, CostUSD: cost}
}

func near(got *float64, want float64) bool { return got != nil && math.Abs(*got-want) < 1e-9 }

func TestCompute_PassRatesCyclesAndCostPerPass(t *testing.T) {
	stats := scoreboard.Compute([]runner.Record{
		rec("A", true, 2, 1), rec("A", true, 4, 1), rec("A", false, -1, 2),
		rec("B", true, 1, 3), rec("B", true, 1, 3), rec("B", true, 3, 3),
		rec("C", false, -1, 5),
	})
	if len(stats) != 3 {
		t.Fatalf("want 3 tasks, got %d", len(stats))
	}
	a, b, c := stats[0], stats[1], stats[2]
	if a.Task != "A" || a.Samples != 3 || a.Passes != 2 || math.Abs(a.PassAt1-2.0/3) > 1e-9 || a.PassPowerK {
		t.Errorf("A = %+v", a)
	}
	if !near(a.MeanCyclesToClean, 3) || !near(a.CostPerPassUSD, 2) {
		t.Errorf("A cycles/cost = %v / %v, want 3 / 2", a.MeanCyclesToClean, a.CostPerPassUSD)
	}
	if b.PassAt1 != 1 || !b.PassPowerK || !near(b.CostPerPassUSD, 3) {
		t.Errorf("B = %+v", b)
	}
	if c.PassAt1 != 0 || c.PassPowerK || c.CostPerPassUSD != nil || c.MeanCyclesToClean != nil {
		t.Errorf("C = %+v: a task with no passes has no cost per pass and no clean cycles", c)
	}
}

func TestMerge_KeepsBestKnownPerTask(t *testing.T) {
	first := scoreboard.Merge(scoreboard.Board{}, scoreboard.Compute([]runner.Record{rec("A", true, 1, 1), rec("A", false, -1, 1)}), "sha1", "tuned1")
	worse := scoreboard.Merge(first, scoreboard.Compute([]runner.Record{rec("A", false, -1, 1), rec("A", false, -1, 1)}), "sha2", "tuned2")
	if worse.Tasks["A"].PassAt1 != 0.5 || worse.Tasks["A"].EvoSHA != "sha1" {
		t.Errorf("a worse run must not replace the best: %+v", worse.Tasks["A"])
	}
	if worse.EvoSHA != "sha2" || worse.TunedSurfaceSHA != "tuned2" {
		t.Errorf("board SHAs must describe the latest update: %+v", worse)
	}
	better := scoreboard.Merge(worse, scoreboard.Compute([]runner.Record{rec("A", true, 1, 1), rec("A", true, 1, 1)}), "sha3", "tuned3")
	if better.Tasks["A"].PassAt1 != 1 || better.Tasks["A"].EvoSHA != "sha3" {
		t.Errorf("a better run must replace it: %+v", better.Tasks["A"])
	}
}

func TestMerge_CheaperWinsATie(t *testing.T) {
	dear := scoreboard.Merge(scoreboard.Board{}, scoreboard.Compute([]runner.Record{rec("A", true, 1, 9)}), "s1", "t")
	cheap := scoreboard.Merge(dear, scoreboard.Compute([]runner.Record{rec("A", true, 1, 2)}), "s2", "t")
	if !near(cheap.Tasks["A"].CostPerPassUSD, 2) {
		t.Errorf("cost per pass = %v, want the cheaper 2", cheap.Tasks["A"].CostPerPassUSD)
	}
}

func TestBoard_RoundTripsThroughJSON(t *testing.T) {
	board := scoreboard.Merge(scoreboard.Board{}, scoreboard.Compute([]runner.Record{rec("A", true, 1, 1)}), "sha", "tuned")
	var buf bytes.Buffer
	if err := board.Write(&buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	back, err := scoreboard.ReadBoard(&buf)
	if err != nil || back.Tasks["A"].Passes != 1 || back.TunedSurfaceSHA != "tuned" {
		t.Fatalf("round trip = %+v, %v", back, err)
	}
	empty, err := scoreboard.ReadBoard(strings.NewReader(""))
	if err != nil || len(empty.Tasks) != 0 {
		t.Fatalf("empty board = %+v, %v", empty, err)
	}
}

func TestReadRecords_ParsesJSONL(t *testing.T) {
	var buf bytes.Buffer
	sink := runner.JSONLSink{Out: &buf}
	for _, r := range []runner.Record{rec("A", true, 1, 1), rec("B", false, -1, 2)} {
		if err := sink.Write(r); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	records, err := scoreboard.ReadRecords(&buf)
	if err != nil || len(records) != 2 || records[1].Task != "B" {
		t.Fatalf("records = %+v, %v", records, err)
	}
	if _, err := scoreboard.ReadRecords(strings.NewReader("{not json}\n")); err == nil {
		t.Error("malformed transcript must be an error")
	}
}

// The held-out report carries aggregates only: neither task names nor file
// contents can appear in what it prints.
func TestHeldOut_NeverPrintsTaskTextOrTranscripts(t *testing.T) {
	secret := runner.Record{
		Task: "SECRET-TASK-NAME", Model: "m", Passed: true, CyclesToClean: 1,
		Files: map[string]string{"main.go": "SECRET-SOURCE-TEXT"}, Ended: "SECRET-ENDED",
	}
	failing := secret
	failing.Task, failing.Passed = "OTHER-SECRET-TASK", false
	agg := scoreboard.HeldOut([]runner.Record{secret, failing})
	line := agg.String()
	for _, leak := range []string{"SECRET", "main.go"} {
		if strings.Contains(line, leak) {
			t.Errorf("held-out report leaks %q: %s", leak, line)
		}
	}
	if agg.Tasks != 2 || agg.Samples != 2 || agg.Passes != 1 || agg.PassAt1 != 0.5 || agg.PassPowerKAll != 0.5 {
		t.Errorf("aggregate = %+v", agg)
	}
}

func TestHeldOutDir_IsOutsideTheRepo(t *testing.T) {
	if got, want := scoreboard.HeldOutDir("/home/u"), filepath.Join("/home/u", ".evo-eval", "heldout"); got != want {
		t.Errorf("HeldOutDir = %q, want %q", got, want)
	}
}
