// This file owns Exec: one managed-state subprocess invocation, skipped when
// the manifest proves it current and verified against its declared Outputs
// when it runs (spec §8.4).

package change

import (
	"context"
	"errors"
	"fmt"

	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
	"github.com/zachbornheimer/evident-output/v2/internal/graph"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// ExecSpec declares one managed-state subprocess invocation (spec §8.4).
// Constructing an ExecSpec performs no I/O; Exec performs the operation.
type ExecSpec struct {
	// Executable is required: a bare name is resolved on PATH, a path
	// containing a separator resolves against Dir.
	Executable string
	// Args is passed to the child literally — no shell, no expansion.
	Args []string
	// Dir is the child's working directory. Empty resolves to the Run's
	// workspace; a relative Dir resolves against the workspace too.
	Dir string
	// Env is merged onto the process's own environment, overriding any
	// name it repeats. Only these explicit entries enter the definition
	// fingerprint (spec §11.4) — inherited variables do not.
	Env map[string]string
	// Basis lists additional Fingerprint inputs whose change invalidates
	// this operation's prior record (spec §11.4). A relative FSPath entry
	// resolves against the workspace, same as File's Path (spec §8.4).
	Basis []freshness.Fingerprint
	// Outputs are paths this invocation produces, relative to Dir. No
	// Outputs means Exec has nothing observable to skip by, so it always
	// runs (spec §8.4).
	Outputs []string
}

// ExecResult is one Exec attempt's immutable outcome (spec §8.4/ZYS-850):
// the smallest inspection surface a caller needs to derive structured
// Problems/Facts from a completed subprocess without taking over capture.
//
// Ran reports whether the child actually reached a terminal exit status —
// false when Exec skipped spawning (a current manifest hit or a dry-run
// plan), matching ProcessOutcome's own "never produced a terminal exit
// status" semantics for the spawn-failure/cancellation case (those return
// only an error, ExecResult zero-valued).
//
// Stdout/Stderr are the Capture tail Exec already retains: sanitized,
// redacted, and bounded (at most 200 completed lines / ~256KiB) — never a
// second unbounded copy, and never the human-facing truncation marker
// DetailTail adds. Truncated reports that the bound dropped earlier output.
// They suit line-oriented diagnostics that tolerate a tail; they are not a
// data channel. A caller that needs a tool's complete machine output (a
// JSON report) has the tool write it to a file and reads that file.
type ExecResult struct {
	Ran       bool
	ExitCode  int
	Stdout    string
	Stderr    string
	Truncated bool
}

// Exec-specific misuse/usage and outcome errors (spec §8.4).
var (
	// ErrExecSpecMissingExecutable is returned when ExecSpec.Executable (Argv[0]) is empty.
	ErrExecSpecMissingExecutable = fmt.Errorf("%w: ExecSpec.Argv must name an executable", graph.ErrInvalidConfig)
	// ErrExecExecutableNotFound is returned when Executable cannot be resolved.
	ErrExecExecutableNotFound = errors.New("evo: Exec could not resolve Executable")
	// ErrExecNonzeroExit is returned when the child exits with a nonzero status.
	ErrExecNonzeroExit = errors.New("evo: Exec exited nonzero")
	// ErrExecOutputMissingAfterSuccess is returned when the child exits 0 but a
	// declared Output does not exist afterward (spec §8.4's verification failure).
	ErrExecOutputMissingAfterSuccess = errors.New("evo: Exec exited 0 but a declared Output is missing")
)

// execTarget is Exec's fully resolved spawn target (spec §8.4's path
// rules), computed once in reconcileExec and threaded through evaluation
// and spawning so no helper re-derives it differently.
type execTarget struct {
	Dir            string
	ExecutablePath string
	Outputs        []string // resolved against Dir, sorted
}

// changes reports whether the Exec declared Outputs: only then is a run of it a change to something Evo can
// name. Without Outputs it ran, and nothing says it changed anything.
func (t execTarget) changes() bool { return len(t.Outputs) > 0 }

// Exec declares/reconciles one managed-state subprocess invocation: it
// skips spawning when a prior record proves the operation is already
// current (matching definition, Basis, and every declared Output digest),
// otherwise runs the child and verifies its declared Outputs afterward. The
// returned ExecResult lets a caller inspect the attempt's exit code and
// captured stdout/stderr without owning capture itself (spec §8.4/ZYS-850);
// ordinary callers that don't parse output may ignore it with
// `_, err := evo.Exec(...)`. ctx must come from a Task's Define callback
// (see graph.ScopeOf) — Exec returns ErrNoTaskContext or ErrTaskClosed
// otherwise, exactly as ScopeOf reports them (already fully descriptive
// sentinels — wrapping would add nothing and would break a bare errors.Is
// check on either).
func Exec(ctx context.Context, spec ExecSpec) (ExecResult, error) {
	scope, scopeErr := graph.BeginOperation[*Run](ctx, fmt.Sprintf("Exec %q", spec.Executable))
	if scopeErr != nil {
		return ExecResult{}, scopeErr
	}
	return scope.Owner().reconcileExec(ctx, scope.Task(), spec)
}

// reconcileExec is Exec's implementation, mirroring reconcileFile's shape
// (spec §11.3-11.6): resolve paths, claim/settle the output freshness
// barrier, consult the manifest for a skip, otherwise spawn and verify.
func (r *Run) reconcileExec(ctx context.Context, id record.TaskID, spec ExecSpec) (ExecResult, error) {
	if spec.Executable == "" {
		return ExecResult{}, r.refuse(ErrExecSpecMissingExecutable)
	}

	dir := r.resolvePath(spec.Dir)
	outputs := resolveExecOutputs(dir, spec.Outputs)

	release, claimErr := r.claimExecOutputs(id, outputs)
	if claimErr != nil {
		return ExecResult{}, claimErr
	}
	defer release()

	executablePath, resolveErr := resolveExecutable(spec.Executable, dir)
	if resolveErr != nil {
		return ExecResult{}, fmt.Errorf("evo: Exec: %w", resolveErr)
	}
	target := execTarget{Dir: dir, ExecutablePath: executablePath, Outputs: outputs}

	eval, evalErr := r.execEvaluate(ctx, id, spec, target)
	if evalErr != nil {
		return ExecResult{}, fmt.Errorf("evo: Exec: %w", evalErr)
	}
	if !eval.Spawns() {
		return ExecResult{Ran: false}, nil
	}
	return r.execRunAndRecord(ctx, id, spec, target, eval)
}

// execEvaluate resolves this call's manifest verdict and narrates it:
// current (skip, forwarding the prior record), stale-but-dry-run (skip,
// planning an Effect), or stale (spawn, carrying the fresh definition/Basis
// the caller commits after a successful run).
func (r *Run) execEvaluate(ctx context.Context, id record.TaskID, spec ExecSpec, target execTarget) (freshness.ExecEvaluation, error) {
	eval, consultErr := r.consultExec(ctx, id, spec, target)
	if consultErr != nil {
		return freshness.ExecEvaluation{}, consultErr
	}
	subject := Subject{Kind: SubjectExec, Name: spec.Executable}
	switch eval.Disposition {
	case freshness.ExecIsCurrent:
		r.host.Observe(Observation{Task: id, Kind: OperationSkippedCurrent, Subject: subject, Reason: eval.Reason})
		carried := eval.Prior
		carried.Carried = true
		r.host.AppendOperation(id, carried)
	case freshness.ExecIsPlanned:
		r.host.Observe(Observation{Task: id, Kind: OperationStarted, Subject: subject, Reason: eval.Reason})
		r.recordChange(id, spec.Executable, target)
		r.host.PlanOperation(id, target.Outputs...)
		r.host.Observe(Observation{Task: id, Kind: OperationFinished, Subject: subject, Changed: target.changes()})
	default:
		r.host.Observe(Observation{Task: id, Kind: OperationStarted, Subject: subject, Reason: eval.Reason})
	}
	return eval, nil
}

// execRunAndRecord spawns the child, verifies its declared Outputs after a
// zero exit, and commits the fresh operation record — the only path that
// actually mutates anything (spec §8.4). The returned ExecResult carries
// the captured attempt (Ran=true) whenever the child reached a terminal
// exit status, even when that attempt then fails verification or exits
// nonzero — only a spawn/cancellation failure (runErr != nil) leaves it
// zero-valued, matching ProcessOutcome's own terminal-status semantics.
func (r *Run) execRunAndRecord(ctx context.Context, id record.TaskID, spec ExecSpec, target execTarget, eval freshness.ExecEvaluation) (ExecResult, error) {
	result, runErr := r.host.Spawn(ctx, id, SpawnCommand{
		Path: target.ExecutablePath,
		Args: append([]string(nil), spec.Args...),
		Dir:  target.Dir,
		Env:  mergedExecEnv(spec.Env),
	})
	if runErr != nil {
		return ExecResult{}, fmt.Errorf("evo: Exec %q: %w", spec.Executable, runErr)
	}
	if result.ExitCode != 0 {
		return result, fmt.Errorf("%w (exit %d): %s", ErrExecNonzeroExit, result.ExitCode, spec.Executable)
	}

	outputRecords, verifyErr := r.observeVerifiedExecOutputs(ctx, id, target.Outputs)
	if verifyErr != nil {
		return result, fmt.Errorf("evo: Exec %q: %w", spec.Executable, verifyErr)
	}

	r.recordChange(id, spec.Executable, target)
	r.host.AppendOperation(id, freshness.OperationRecord{
		Kind:                  freshness.OperationKindExec,
		DefinitionFingerprint: eval.DefinitionFingerprint,
		Basis:                 eval.Basis,
		Outputs:               outputRecords,
	})
	r.host.Observe(Observation{Task: id, Kind: OperationFinished, Subject: Subject{Kind: SubjectExec, Name: spec.Executable}, Changed: target.changes()})
	return result, nil
}

// recordChange adds the ledger row for an Exec that declared Outputs. One that declared none ran but has no
// result Evo can call a change, so it records only the operation (CONTRACT section 3).
func (r *Run) recordChange(id record.TaskID, executable string, target execTarget) {
	if target.changes() {
		r.recordEntry(id, record.NamedEntry("run", executable))
	}
}
