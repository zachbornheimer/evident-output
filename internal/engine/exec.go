package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/freshness"
	"github.com/zachbornheimer/evident-output/internal/fs"
	"github.com/zachbornheimer/evident-output/internal/process"
	"github.com/zachbornheimer/evident-output/internal/record"
	"github.com/zachbornheimer/evident-output/internal/wire"
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
	// ErrExecSpecMissingExecutable is returned when ExecSpec.Executable is empty.
	ErrExecSpecMissingExecutable = errors.New("evo: ExecSpec.Executable is required")
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

// Exec declares/reconciles one managed-state subprocess invocation: it
// skips spawning when a prior record proves the operation is already
// current (matching definition, Basis, and every declared Output digest),
// otherwise runs the child and verifies its declared Outputs afterward. The
// returned ExecResult lets a caller inspect the attempt's exit code and
// captured stdout/stderr without owning capture itself (spec §8.4/ZYS-850);
// ordinary callers that don't parse output may ignore it with
// `_, err := evo.Exec(...)`. ctx must come from a Task's Define callback
// (see taskScope) — Exec returns ErrNoTaskContext or ErrTaskClosed
// otherwise, exactly as taskScope reports them (already fully descriptive
// sentinels — wrapping would add nothing and would break a bare errors.Is
// check on either).
func Exec(ctx context.Context, spec ExecSpec) (ExecResult, error) {
	task, scopeErr := beginOperation(ctx, fmt.Sprintf("Exec %q", spec.Executable))
	if scopeErr != nil {
		return ExecResult{}, scopeErr
	}
	return task.out.reconcileExec(ctx, task.id, spec)
}

// reconcileExec is Exec's implementation, mirroring reconcileFile's shape
// (spec §11.3-11.6): resolve paths, claim/settle the output freshness
// barrier, consult the manifest for a skip, otherwise spawn and verify.
func (o *Output) reconcileExec(ctx context.Context, taskID string, spec ExecSpec) (ExecResult, error) {
	if spec.Executable == "" {
		return ExecResult{}, ErrExecSpecMissingExecutable
	}

	dir := o.resolveWorkspacePath(spec.Dir)
	outputs := resolveExecOutputs(dir, spec.Outputs)

	release, claimErr := o.claimExecOutputs(taskID, outputs)
	if claimErr != nil {
		return ExecResult{}, claimErr
	}
	defer release()

	executablePath, resolveErr := o.resolveExecutable(spec.Executable, dir)
	if resolveErr != nil {
		return ExecResult{}, fmt.Errorf("evo: Exec: %w", resolveErr)
	}
	target := execTarget{Dir: dir, ExecutablePath: executablePath, Outputs: outputs}

	eval, evalErr := o.execEvaluate(ctx, taskID, spec, target)
	if evalErr != nil {
		return ExecResult{}, fmt.Errorf("evo: Exec: %w", evalErr)
	}
	if !eval.Spawns() {
		return ExecResult{Ran: false}, nil
	}
	return o.execRunAndRecord(ctx, taskID, spec, target, eval)
}

// execEvaluate resolves this call's manifest verdict and narrates it:
// current (skip, forwarding the prior record), stale-but-dry-run (skip,
// planning an Effect), or stale (spawn, carrying the fresh definition/Basis
// the caller commits after a successful run).
func (o *Output) execEvaluate(ctx context.Context, taskID string, spec ExecSpec, target execTarget) (freshness.ExecEvaluation, error) {
	eval, consultErr := o.execConsultManifest(ctx, taskID, spec, target)
	if consultErr != nil {
		return freshness.ExecEvaluation{}, consultErr
	}
	switch eval.Disposition {
	case freshness.ExecIsCurrent:
		o.mu.Lock()
		o.emitWireEventLocked(wire.EventOperationSkippedCurrent, taskID, map[string]any{
			"kind": freshness.OperationKindExec, "executable": spec.Executable, "reason": eval.Reason,
		})
		o.mu.Unlock()
		if !o.DryRun() {
			o.mu.Lock()
			o.appendManifestOperationLocked(taskID, eval.Prior)
			o.mu.Unlock()
		}
	case freshness.ExecIsPlanned:
		o.emitExecStarted(taskID, spec, eval.Reason)
		o.recordExecEffect(taskID, spec.Executable)
		o.mu.Lock()
		o.emitWireEventLocked(wire.EventOperationFinished, taskID, map[string]any{
			"kind": freshness.OperationKindExec, "executable": spec.Executable, "changed": true,
		})
		o.mu.Unlock()
	default:
		o.emitExecStarted(taskID, spec, eval.Reason)
	}
	return eval, nil
}

// emitExecStarted announces that Exec reconciles because of reason.
func (o *Output) emitExecStarted(taskID string, spec ExecSpec, reason string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitWireEventLocked(wire.EventOperationStarted, taskID, map[string]any{
		"kind": freshness.OperationKindExec, "executable": spec.Executable, "reason": reason,
	})
}

// execRunAndRecord spawns the child, verifies its declared Outputs after a
// zero exit, and commits the fresh operation record — the only path that
// actually mutates anything (spec §8.4). The returned ExecResult carries
// the captured attempt (Ran=true) whenever the child reached a terminal
// exit status, even when that attempt then fails verification or exits
// nonzero — only a spawn/cancellation failure (runErr != nil) leaves it
// zero-valued, matching ProcessOutcome's own terminal-status semantics.
func (o *Output) execRunAndRecord(ctx context.Context, taskID string, spec ExecSpec, target execTarget, eval freshness.ExecEvaluation) (ExecResult, error) {
	result, runErr := o.spawnExec(ctx, taskID, spec, target)
	if runErr != nil {
		return ExecResult{}, fmt.Errorf("evo: Exec %q: %w", spec.Executable, runErr)
	}
	if result.ExitCode != 0 {
		return result, fmt.Errorf("%w (exit %d): %s", ErrExecNonzeroExit, result.ExitCode, spec.Executable)
	}

	outputRecords, verifyErr := o.observeVerifiedExecOutputs(ctx, taskID, target.Outputs)
	if verifyErr != nil {
		return result, fmt.Errorf("evo: Exec %q: %w", spec.Executable, verifyErr)
	}

	o.recordExecEffect(taskID, spec.Executable)
	rec := freshness.OperationRecord{
		Kind:                  freshness.OperationKindExec,
		DefinitionFingerprint: eval.DefinitionFingerprint,
		Basis:                 eval.Basis,
		Outputs:               outputRecords,
	}
	o.mu.Lock()
	o.appendManifestOperationLocked(taskID, rec)
	o.emitWireEventLocked(wire.EventOperationFinished, taskID, map[string]any{
		"kind": freshness.OperationKindExec, "executable": spec.Executable, "changed": true,
	})
	o.mu.Unlock()
	return result, nil
}

// observeVerifiedExecOutputs wraps verifiedExecOutputs with one
// tracked_resource.observed event per declared output (spec §38), mirroring
// File's single tracked_resource.observed for its one managed path — Exec
// has as many tracked resources as it has declared Outputs.
func (o *Output) observeVerifiedExecOutputs(ctx context.Context, taskID string, outputs []string) ([]freshness.OutputRecord, error) {
	for _, out := range outputs {
		_, statErr := fs.Stat(out)
		o.mu.Lock()
		o.emitWireEventLocked(wire.EventTrackedResourceObserved, taskID, map[string]any{
			"kind": "exec-output", "path": out, "exists": statErr == nil,
		})
		o.mu.Unlock()
	}
	return verifiedExecOutputs(ctx, outputs)
}

// resolveExecutable resolves ExecSpec.Executable to an absolute path: a
// name containing a path separator resolves against dir like a shell would
// (spec §8.4 does not PATH-search an explicit path); a bare name is
// resolved on PATH via the lookPath facade.
func (o *Output) resolveExecutable(executable, dir string) (string, error) {
	if process.IsExplicitPath(executable) {
		return resolvePathAgainst(dir, executable), nil
	}
	resolved, lookErr := lookPath(executable)
	if lookErr != nil {
		return "", fmt.Errorf("%w: %s", ErrExecExecutableNotFound, executable)
	}
	return resolved, nil
}

// lookPath is the facade resolveExecutable reads PATH through, instead of
// exec.LookPath directly (facade rule).
var lookPath = process.LookPath

// verifiedExecOutputs re-inspects every declared output after a successful
// (exit 0) run (spec §8.4: "after exit 0 every declared output must exist
// and fingerprint, else verification failure") and returns their tracked
// output records.
func verifiedExecOutputs(ctx context.Context, outputs []string) ([]freshness.OutputRecord, error) {
	records := make([]freshness.OutputRecord, len(outputs))
	for i, out := range outputs {
		if _, statErr := fs.Stat(out); statErr != nil {
			return nil, fmt.Errorf("%w: %s", ErrExecOutputMissingAfterSuccess, out)
		}
		digest, digestErr := freshness.PathOutputDigest(ctx, out)
		if digestErr != nil {
			return nil, fmt.Errorf("evo: Exec output %q: %w", out, digestErr)
		}
		records[i] = freshness.OutputRecord{Kind: "exec-output", Path: out, Digest: digest}
	}
	return records, nil
}

// recordExecEffect records Exec's planned (dry-run) or committed (applied)
// Effect under taskID's own ledger section — the same routing recordFileEffect
// uses for File (spec §8.4/§27/§51).
func (o *Output) recordExecEffect(taskID, displayExecutable string) {
	o.recordLedgerEntry(taskID, record.NamedEntry("run", displayExecutable))
}
