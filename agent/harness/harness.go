// Package harness evaluates agent-assistance scenarios (§30.9).
package harness

import (
	"strings"

	"github.com/zachbornheimer/evident-output/agent/catalog"
	"github.com/zachbornheimer/evident-output/agent/review"
)

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
}

// Result is the outcome of running a scenario.
type Result struct {
	ID              string
	Passed          bool
	MissingRules    []string
	RecheckRequired bool
	GuidanceOK      bool
	Detail          string
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
  out := evo.New()
  fmt.Printf("hi")
  _ = out
}
`,
			MustDetect: []string{"STREAM-003"},
			GuidanceID: "streams",
		},
		{
			ID:          "redundant-start",
			Description: "detect redundant Start",
			BadSource: `package p
import evo "github.com/zachbornheimer/evident-output"
func f() {
  out := evo.New()
  t := out.Task("x")
  t.Start()
}
`,
			MustDetect: []string{"API-006"},
			GuidanceID: "tasks",
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

// Run executes all default scenarios.
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
