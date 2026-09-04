// Package harness evaluates agent-assistance scenarios (§30.9, MCP-022/049).
package harness

import (
	"fmt"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/agent/catalog"
	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// DefaultMaxCycles bounds the repair–recheck loop (MCP-022).
const DefaultMaxCycles = 8

// Scenario is one fixed agent-effectiveness case.
type Scenario struct {
	ID          string
	Description string
	// BadSource is intentionally incorrect Go using evo poorly.
	BadSource string
	// MustDetect lists rule IDs that review must report on BadSource.
	MustDetect []string
	// GuidanceID is a catalog entry that should exist for this scenario.
	GuidanceID string
	// Repairable: when true, RunRepairLoop must reach recheck_required=false.
	Repairable bool
}

// Result is the outcome of running a scenario.
type Result struct {
	ID              string
	Passed          bool
	MissingRules    []string
	RecheckRequired bool
	GuidanceOK      bool
	Cycles          int
	Clean           bool // true when final review has recheck_required=false
	Detail          string
}

// RepairLoopResult is the outcome of an iterative repair–recheck run (MCP-022).
type RepairLoopResult struct {
	Initial       review.Result
	Final         review.Result
	Cycles        int
	ReachedClean  bool
	StoppedReason string
	Sources       []string // source after each cycle (including initial)
}

// DefaultScenarios returns the versioned suite from the architecture spec.
func DefaultScenarios() []Scenario {
	return []Scenario{
		{
			ID:          "migrate-fmt",
			Description: "detect fmt.Printf alongside evo",
			BadSource: `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func f() {
  out := evo.Init(evo.Config{Options: []evo.Option{}})
  fmt.Printf("hi")
  _ = out
}
`,
			MustDetect: []string{"STREAM-003"},
			GuidanceID: "streams",
			Repairable: true,
		},
		{
			ID:          "redundant-start",
			Description: "detect redundant Start",
			BadSource: `package p
import evo "github.com/zachbornheimer/evident-output"
func f() {
  out := evo.Init(evo.Config{Options: []evo.Option{}})
  t := out.Task("x")
  t.Start()
}
`,
			MustDetect: []string{"API-006"},
			GuidanceID: "tasks",
			Repairable: true,
		},
		{
			ID:          "blocked-as-error",
			Description: "detect blocked item returned as application error (MCP-014)",
			BadSource: `package p
import (
  "errors"
  evo "github.com/zachbornheimer/evident-output"
)
func check() error {
  out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("repo")}})
  defer out.Close()
  out.Task("working tree").Block("dirty")
  return errors.New("dirty")
}
`,
			MustDetect: []string{"DOM-011"},
			GuidanceID: "common-api",
			Repairable: true,
		},
		{
			ID:          "common-api-guidance",
			Description: "guidance catalog has common-api",
			BadSource:   `package p`,
			MustDetect:  nil,
			GuidanceID:  "common-api",
		},
	}
}

// Run executes all default scenarios (detect-only pass criteria).
func Run() []Result {
	var out []Result
	for _, s := range DefaultScenarios() {
		out = append(out, RunOne(s))
	}
	return out
}

// RunOne evaluates a single scenario against shipped review+catalog.
func RunOne(s Scenario) Result {
	res := Result{ID: s.ID, GuidanceOK: true}
	if s.GuidanceID != "" {
		found, missing := catalog.Get([]string{s.GuidanceID})
		if len(found) == 0 || len(missing) > 0 {
			res.GuidanceOK = false
			res.Detail = "missing guidance " + s.GuidanceID
		}
	}
	if s.BadSource != "" && len(s.MustDetect) > 0 {
		rev := review.GoSource(s.ID+".go", s.BadSource)
		res.RecheckRequired = rev.RecheckRequired
		have := map[string]bool{}
		for _, f := range rev.Findings {
			have[f.RuleID] = true
		}
		for _, need := range s.MustDetect {
			if !have[need] {
				res.MissingRules = append(res.MissingRules, need)
			}
		}
	}
	res.Passed = res.GuidanceOK && len(res.MissingRules) == 0
	if !res.Passed && res.Detail == "" {
		res.Detail = "missing rules: " + strings.Join(res.MissingRules, ",")
	}
	return res
}

// RunRepairLoop applies mechanical fixes and re-reviews until recheck_required
// is false or maxCycles is exhausted (MCP-022 / MCP-049).
func RunRepairLoop(src string, maxCycles int) RepairLoopResult {
	if maxCycles <= 0 {
		maxCycles = DefaultMaxCycles
	}
	out := RepairLoopResult{Sources: []string{src}}
	out.Initial = review.GoSource("loop.go", src)
	cur := src
	for cycle := 0; cycle < maxCycles; cycle++ {
		rev := review.GoSource("loop.go", cur)
		out.Final = rev
		out.Cycles = cycle + 1
		if !rev.RecheckRequired {
			out.ReachedClean = true
			out.StoppedReason = "recheck_required=false"
			return out
		}
		next, changed := ApplyMechanicalFixes(cur, rev.Findings)
		if !changed {
			out.StoppedReason = "no mechanical fix available; still recheck_required"
			return out
		}
		cur = next
		out.Sources = append(out.Sources, cur)
	}
	out.Final = review.GoSource("loop.go", cur)
	if !out.Final.RecheckRequired {
		out.ReachedClean = true
		out.StoppedReason = "recheck_required=false"
		return out
	}
	out.StoppedReason = fmt.Sprintf("max cycles %d exhausted; recheck_required still true", maxCycles)
	return out
}

// ApplyMechanicalFixes performs deterministic, safe textual repairs for known
// rule IDs. Returns the new source and whether any change was made.
func ApplyMechanicalFixes(src string, findings []review.Finding) (string, bool) {
	changed := false
	out := src
	seen := map[string]bool{}
	for _, f := range findings {
		if seen[f.RuleID] {
			continue
		}
		seen[f.RuleID] = true
		switch f.RuleID {
		case "API-006":
			// Remove lines that are only t.Start() / x.Start().
			var b strings.Builder
			for _, line := range strings.Split(out, "\n") {
				trim := strings.TrimSpace(line)
				if strings.HasSuffix(trim, ".Start()") || strings.HasSuffix(trim, ".Start();") {
					changed = true
					continue
				}
				b.WriteString(line)
				b.WriteByte('\n')
			}
			out = strings.TrimSuffix(b.String(), "\n")
			if strings.HasSuffix(src, "\n") && !strings.HasSuffix(out, "\n") {
				out += "\n"
			}
		case "STREAM-003":
			var b strings.Builder
			for _, line := range strings.Split(out, "\n") {
				trim := strings.TrimSpace(line)
				if strings.Contains(trim, "fmt.Print") || strings.Contains(trim, "fmt.Fprint") {
					// Drop contaminating print; agent would replace with out.Line.
					changed = true
					continue
				}
				// Drop unused fmt import if no fmt. remains.
				b.WriteString(line)
				b.WriteByte('\n')
			}
			out = strings.TrimSuffix(b.String(), "\n")
			if !strings.Contains(out, "fmt.") {
				out = removeImport(out, `"fmt"`)
			}
		case "DOM-011":
			// Replace application-error returns after Block with return nil.
			var b strings.Builder
			for _, line := range strings.Split(out, "\n") {
				trim := strings.TrimSpace(line)
				if strings.Contains(trim, "return errors.New(") ||
					strings.Contains(trim, "return fmt.Errorf(") {
					indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
					b.WriteString(indent + "return nil // blocked is presentation outcome\n")
					changed = true
					continue
				}
				if trim == "return err" {
					indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
					b.WriteString(indent + "return nil\n")
					changed = true
					continue
				}
				b.WriteString(line)
				b.WriteByte('\n')
			}
			out = strings.TrimSuffix(b.String(), "\n")
		}
	}
	return out, changed
}

func removeImport(src, pathLit string) string {
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		if strings.Contains(line, pathLit) {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// RunAllRepairable runs repair loops for every Repairable default scenario
// and requires each to reach recheck_required=false (MCP-022 + MCP-049).
func RunAllRepairable() []Result {
	var out []Result
	for _, s := range DefaultScenarios() {
		if !s.Repairable {
			continue
		}
		loop := RunRepairLoop(s.BadSource, DefaultMaxCycles)
		r := Result{
			ID:              s.ID,
			Cycles:          loop.Cycles,
			RecheckRequired: loop.Final.RecheckRequired,
			Clean:           loop.ReachedClean,
			Passed:          loop.ReachedClean,
			Detail:          loop.StoppedReason,
		}
		// Also require initial detection of MustDetect.
		init := review.GoSource(s.ID+".go", s.BadSource)
		have := map[string]bool{}
		for _, f := range init.Findings {
			have[f.RuleID] = true
		}
		for _, need := range s.MustDetect {
			if !have[need] {
				r.MissingRules = append(r.MissingRules, need)
				r.Passed = false
			}
		}
		out = append(out, r)
	}
	return out
}
