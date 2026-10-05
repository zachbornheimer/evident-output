package evaltask

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

const (
	envOutput          = "EVO_OUTPUT"
	outputJSON         = "json"
	outputJSONL        = "jsonl"
	binaryName         = "answer.bin"
	cycleMessage       = "After dependency cycle"
	runTimeout         = 30 * time.Second
	buildTargetAnswer  = "./" + answerDirName
	buildTargetFixture = "./fixture"
)

// offlineGoEnv keeps the build hermetic: no proxy, no workspace, and a
// go.mod the toolchain may complete from the local module cache.
var offlineGoEnv = []string{"GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off"}

// Report is the score of one candidate against one task.
type Report struct {
	Compiles    bool
	BuildOutput string
	// ReviewClean is review's recheck_required=false with no findings.
	ReviewClean    bool
	ReviewFindings []string // rule IDs
	// TopologyMatches is the run's tree and order equal to the task's
	// expectation.
	TopologyMatches bool
	TopologyDiff    string
	OrderViolations []string
	// BannedPatterns are the detectors that fired on the candidate source.
	BannedPatterns []string
	// Cycles is true when the run reported an After dependency cycle.
	Cycles bool
}

// Passed is a compiling, review-clean, correctly shaped candidate with no
// banned pattern and no cycle.
func (r Report) Passed() bool {
	return r.Compiles && r.ReviewClean && r.TopologyMatches && len(r.BannedPatterns) == 0 && !r.Cycles
}

// Grader scores candidates. RepoRoot is the library checkout candidates
// build against.
type Grader struct {
	RepoRoot string
	Runner   Runner
}

// Grade scores candidate against task inside workDir, an empty directory the
// caller owns and removes: source checks, a build, and a run compared with
// the task's expected topology.
func (g Grader) Grade(ctx context.Context, task Task, candidate fs.FS, workDir string) (Report, error) {
	return g.grade(ctx, task, candidate, workDir, true)
}

// GradeSource is Grade without the link and the run: source checks and a
// compile check only, so TopologyMatches stays false. The replay uses it for
// traps, whose rejection does not depend on how they behave.
func (g Grader) GradeSource(ctx context.Context, task Task, candidate fs.FS, workDir string) (Report, error) {
	return g.grade(ctx, task, candidate, workDir, false)
}

func (g Grader) grade(ctx context.Context, task Task, candidate fs.FS, workDir string, run bool) (Report, error) {
	fixture, err := task.FixtureFS()
	if err != nil {
		return Report{}, fmt.Errorf("grade task %s: %w", task.ID, err)
	}
	sandbox, err := NewSandbox(workDir, g.RepoRoot, fixture, candidate)
	if err != nil {
		return Report{}, fmt.Errorf("grade task %s: %w", task.ID, err)
	}
	var report Report
	if err := g.scoreSource(task, candidate, sandbox, &report); err != nil {
		return Report{}, err
	}
	if err := g.build(ctx, sandbox, run, &report); err != nil {
		return Report{}, fmt.Errorf("grade task %s: %w", task.ID, err)
	}
	if !report.Compiles || !run {
		return report, nil
	}
	if err := g.scoreRun(ctx, task, sandbox, &report); err != nil {
		return Report{}, err
	}
	return report, nil
}

func (g Grader) scoreSource(task Task, candidate fs.FS, sandbox Sandbox, report *Report) error {
	banned, err := BannedPatterns(candidate, task.Expect.BannedPatterns)
	if err != nil {
		return fmt.Errorf("grade task %s: %w", task.ID, err)
	}
	report.BannedPatterns = banned
	result, err := review.GoDirectory(sandbox.AnswerDir())
	if err != nil {
		return fmt.Errorf("grade task %s: %w", task.ID, err)
	}
	report.ReviewClean = !result.RecheckRequired && len(result.Findings) == 0
	for _, finding := range result.Findings {
		report.ReviewFindings = append(report.ReviewFindings, finding.RuleID)
	}
	return nil
}

// build links the candidate when it will be run. Naming two packages makes
// go build compile and discard, which is all a source-only grade needs.
func (g Grader) build(ctx context.Context, sandbox Sandbox, link bool, report *Report) error {
	args := []string{"build", buildTargetFixture, buildTargetAnswer}
	if link {
		args = []string{"build", "-ldflags=-s -w", "-o", binaryName, buildTargetAnswer}
	}
	res, err := g.Runner.Run(ctx, Command{Dir: sandbox.Root, Env: offlineGoEnv, Name: "go", Args: args})
	if err != nil {
		return fmt.Errorf("build candidate: %w", err)
	}
	report.Compiles = res.ExitCode == 0
	report.BuildOutput = strings.TrimSpace(res.Stderr + res.Stdout)
	return nil
}

func (g Grader) scoreRun(ctx context.Context, task Task, sandbox Sandbox, report *Report) error {
	document, err := g.execute(ctx, sandbox, outputJSON)
	if err != nil {
		return fmt.Errorf("grade task %s: %w", task.ID, err)
	}
	stream, err := g.execute(ctx, sandbox, outputJSONL)
	if err != nil {
		return fmt.Errorf("grade task %s: %w", task.ID, err)
	}
	report.Cycles = strings.Contains(document.Stdout+document.Stderr+stream.Stdout+stream.Stderr, cycleMessage)
	tree, err := ParseTopology([]byte(document.Stdout))
	if err != nil {
		report.TopologyDiff = fmt.Sprintf("run produced no evo.run document (exit %d): %v\n%s", document.ExitCode, err, document.Stderr)
		return nil
	}
	report.TopologyDiff = DiffTopology(tree, task.Expect.Tree)
	order, err := ParseRunOrder([]byte(stream.Stdout))
	if err != nil {
		return fmt.Errorf("grade task %s: %w", task.ID, err)
	}
	report.OrderViolations = order.Violations(task.Expect.Order)
	report.TopologyMatches = report.TopologyDiff == "" && len(report.OrderViolations) == 0
	return nil
}

func (g Grader) execute(ctx context.Context, sandbox Sandbox, format string) (CommandResult, error) {
	runCtx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()
	res, err := g.Runner.Run(runCtx, Command{
		Dir: sandbox.Root, Env: []string{envOutput + "=" + format},
		Name: filepath.Join(sandbox.Root, binaryName),
	})
	if err != nil {
		return res, fmt.Errorf("run candidate with %s=%s: %w", envOutput, format, err)
	}
	return res, nil
}
