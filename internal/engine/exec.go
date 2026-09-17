package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/fingerprint"
	"github.com/zachbornheimer/evident-output/internal/manifest"
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
	Basis []fingerprint.Fingerprint
	// Outputs are paths this invocation produces, relative to Dir. No
	// Outputs means Exec has nothing observable to skip by, so it always
	// runs (spec §8.4).
	Outputs []string
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

// execEvaluation is reconcileExec's manifest verdict: either the operation
// is already handled (Skip true — a current hit, or a dry-run plan) or it
// needs to actually spawn, in which case DefinitionFingerprint/Basis are
// what the post-spawn success record commits.
type execEvaluation struct {
	Skip                  bool
	DefinitionFingerprint string
	Basis                 []manifest.BasisRecord
}

// Exec declares/reconciles one managed-state subprocess invocation: it
// skips spawning when a prior record proves the operation is already
// current (matching definition, Basis, and every declared Output digest),
// otherwise runs the child and verifies its declared Outputs afterward.
// ctx must come from a Task's Define callback (see taskScope) — Exec
// returns ErrNoTaskContext or ErrTaskClosed otherwise, exactly as taskScope
// reports them (already fully descriptive sentinels — wrapping would add
// nothing and would break a bare errors.Is check on either).
func Exec(ctx context.Context, spec ExecSpec) error {
	task, scopeErr := taskScope(ctx)
	if scopeErr != nil {
		return scopeErr
	}
	return task.out.reconcileExec(ctx, task.id, spec)
}

// reconcileExec is Exec's implementation, mirroring reconcileFile's shape
// (spec §11.3-11.6): resolve paths, claim/settle the output freshness
// barrier, consult the manifest for a skip, otherwise spawn and verify.
func (o *Output) reconcileExec(ctx context.Context, taskID string, spec ExecSpec) error {
	if spec.Executable == "" {
		return ErrExecSpecMissingExecutable
	}
	if cancelErr := o.recordCancelledExec(ctx, spec); cancelErr != nil {
		return cancelErr
	}

	dir := o.resolveWorkspacePath(spec.Dir)
	outputs := resolveExecOutputs(dir, spec.Outputs)

	release, claimErr := o.claimExecOutputs(taskID, outputs)
	if claimErr != nil {
		return claimErr
	}
	defer release()

	executablePath, resolveErr := o.resolveExecutable(spec.Executable, dir)
	if resolveErr != nil {
		return fmt.Errorf("evo: Exec: %w", resolveErr)
	}
	target := execTarget{Dir: dir, ExecutablePath: executablePath, Outputs: outputs}

	eval, evalErr := o.execEvaluate(ctx, taskID, spec, target)
	if evalErr != nil {
		return fmt.Errorf("evo: Exec: %w", evalErr)
	}
	if eval.Skip {
		return nil
	}
	return o.execRunAndRecord(ctx, taskID, spec, target, eval)
}

// recordCancelledExec reports ctx's error as misuse (spec: a cancelled Run
// must never look like it silently succeeded) before Exec does anything
// else — the same guard reconcileFile applies for File.
func (o *Output) recordCancelledExec(ctx context.Context, spec ExecSpec) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		wrapped := fmt.Errorf("evo: Exec %q: %w", spec.Executable, ctxErr)
		o.mu.Lock()
		o.recordMisuse(wrapped)
		o.mu.Unlock()
		return wrapped
	}
	return nil
}

// execEvaluate resolves this call's manifest verdict: current (skip,
// forwarding the prior record), stale-but-dry-run (skip, planning an
// Effect), or stale (spawn, carrying the fresh definition/Basis the caller
// commits after a successful run).
func (o *Output) execEvaluate(ctx context.Context, taskID string, spec ExecSpec, target execTarget) (execEvaluation, error) {
	current, prior, defFingerprint, basis, consultErr := o.execConsultManifest(ctx, taskID, spec, target)
	if consultErr != nil {
		return execEvaluation{}, consultErr
	}
	if current {
		if !o.DryRun() {
			o.mu.Lock()
			o.appendManifestOperationLocked(taskID, prior)
			o.mu.Unlock()
		}
		return execEvaluation{Skip: true}, nil
	}
	if o.DryRun() {
		o.recordExecEffect(taskID, spec.Executable)
		return execEvaluation{Skip: true}, nil
	}
	return execEvaluation{DefinitionFingerprint: defFingerprint, Basis: basis}, nil
}

// execRunAndRecord spawns the child, verifies its declared Outputs after a
// zero exit, and commits the fresh operation record — the only path that
// actually mutates anything (spec §8.4).
func (o *Output) execRunAndRecord(ctx context.Context, taskID string, spec ExecSpec, target execTarget, eval execEvaluation) error {
	outcome, runErr := o.spawnExec(ctx, taskID, spec, target)
	if runErr != nil {
		return fmt.Errorf("evo: Exec %q: %w", spec.Executable, runErr)
	}
	if outcome.ExitCode != 0 {
		return fmt.Errorf("%w (exit %d): %s", ErrExecNonzeroExit, outcome.ExitCode, spec.Executable)
	}

	outputRecords, verifyErr := verifiedExecOutputs(ctx, target.Outputs)
	if verifyErr != nil {
		return fmt.Errorf("evo: Exec %q: %w", spec.Executable, verifyErr)
	}

	o.recordExecEffect(taskID, spec.Executable)
	rec := manifest.OperationRecord{
		Kind:                  "exec",
		DefinitionFingerprint: eval.DefinitionFingerprint,
		Basis:                 eval.Basis,
		Outputs:               outputRecords,
	}
	o.mu.Lock()
	o.appendManifestOperationLocked(taskID, rec)
	o.mu.Unlock()
	return nil
}

// resolveExecutable resolves ExecSpec.Executable to an absolute path: a
// name containing a path separator resolves against dir like a shell would
// (spec §8.4 does not PATH-search an explicit path); a bare name is
// resolved on PATH via the lookPath facade.
func (o *Output) resolveExecutable(executable, dir string) (string, error) {
	if strings.ContainsRune(executable, os.PathSeparator) {
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
var lookPath = exec.LookPath

// verifiedExecOutputs re-inspects every declared output after a successful
// (exit 0) run (spec §8.4: "after exit 0 every declared output must exist
// and fingerprint, else verification failure") and returns their tracked
// output records.
func verifiedExecOutputs(ctx context.Context, outputs []string) ([]manifest.OutputRecord, error) {
	records := make([]manifest.OutputRecord, len(outputs))
	for i, out := range outputs {
		if _, statErr := os.Stat(out); statErr != nil {
			return nil, fmt.Errorf("%w: %s", ErrExecOutputMissingAfterSuccess, out)
		}
		digest, digestErr := pathOutputDigest(ctx, out)
		if digestErr != nil {
			return nil, fmt.Errorf("evo: Exec output %q: %w", out, digestErr)
		}
		records[i] = manifest.OutputRecord{Kind: "exec-output", Path: out, Digest: digest}
	}
	return records, nil
}

// recordExecEffect records Exec's planned (dry-run) or committed (applied)
// Effect under taskID's own ledger section — the same routing recordFileEffect
// uses for File (spec §8.4/§27/§51).
func (o *Output) recordExecEffect(taskID, displayExecutable string) {
	(&TaskHandle{out: o, id: taskID}).RecordName("run", displayExecutable)
}
