package harness_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/agent/harness"
	"github.com/zachbornheimer/evident-output/agent/review"
)

func TestAgentHarness_DefaultScenariosPass(t *testing.T) {
	results := harness.Run()
	if len(results) == 0 {
		t.Fatal("no scenarios")
	}
	for _, r := range results {
		if !r.Passed {
			t.Errorf("scenario %s failed: %s missing=%v", r.ID, r.Detail, r.MissingRules)
		}
	}
}

func TestMCP022_RepairLoopReachesClean(t *testing.T) {
	// Known-bad sample → mechanical fix → recheck_required=false in bounded cycles.
	bad := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func f() {
  out := evo.New()
  t := out.Task("x")
  t.Start()
  fmt.Printf("hi")
  _ = out
}
`
	// Prove RED path first: bad source requires recheck.
	init := review.GoSource("bad.go", bad)
	if !init.RecheckRequired {
		t.Fatal("expected recheck_required on bad source")
	}
	loop := harness.RunRepairLoop(bad, harness.DefaultMaxCycles)
	if !loop.ReachedClean {
		t.Fatalf("did not reach clean: cycles=%d reason=%s final=%+v", loop.Cycles, loop.StoppedReason, loop.Final)
	}
	if loop.Final.RecheckRequired {
		t.Fatal("final still recheck_required")
	}
	if loop.Cycles < 1 || loop.Cycles > harness.DefaultMaxCycles {
		t.Fatalf("cycles=%d", loop.Cycles)
	}
	if loop.StoppedReason != "recheck_required=false" {
		t.Fatalf("stop reason: %s", loop.StoppedReason)
	}
}

func TestMCP049_StopOnlyWhenRecheckFalse(t *testing.T) {
	// Agent stopping condition: loop stops only when recheck_required=false.
	for _, r := range harness.RunAllRepairable() {
		if !r.Passed || !r.Clean {
			t.Errorf("scenario %s did not stop clean: %+v", r.ID, r)
		}
		if r.RecheckRequired {
			t.Errorf("scenario %s stopped with recheck_required still true", r.ID)
		}
	}
}

func TestMCP022_NoFixStopsWithReason(t *testing.T) {
	// Unfixable finding should not claim clean.
	// API-000 parse error cannot be mechanically fixed.
	src := `package p
func f( {
`
	loop := harness.RunRepairLoop(src, 3)
	if loop.ReachedClean {
		t.Fatal("parse errors should not reach clean via mechanical fixes")
	}
	if loop.StoppedReason == "" {
		t.Fatal("expected stop reason")
	}
}
