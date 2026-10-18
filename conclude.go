package evo

import (
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

func concludeInterruptible(out *Run, interrupted *atomic.Bool, runErr error) Result {
	if interrupted.Load() {
		return concludeCancelled(out, runErr)
	}
	return concludeRun(out, runErr)
}

func concludeRunAfterExit(out *Run, interrupted *atomic.Bool, returned bool, runErr error) Result {
	if !returned {
		runErr = errRunExitedWithoutReturn
	}
	return concludeInterruptible(out, interrupted, runErr)
}

func concludeRun(out *Run, runErr error) Result {
	if runErr != nil && !out.anyFailed() {
		// Record the application error only when no entity already Failed (avoids a duplicate Fail row).
		out.end(runErr)
	}
	finishErr := out.finish()
	closeErr := out.close()
	conclusion := out.conclusion()
	// Bookkeeping misuse is already in the Conclusion. A renderer or write failure surfaces after
	// the band is flushed, so it escalates an otherwise-OK exit code here.
	if conclusion.exitCode == exitOK && (errors.Is(finishErr, errRenderer) || errors.Is(closeErr, errRenderer)) {
		conclusion.exitCode = exitFailed
	}
	return Result{Conclusion: conclusion, Err: out.resultErr(runErr)}
}

// concludeCancelled: the recorded Cancel's Conclusion alone yields ExitCancelled; runErr rides as Result.Err.
func concludeCancelled(out *Run, runErr error) Result {
	_ = out.finish()
	_ = out.close()
	return Result{Conclusion: out.conclusion(), Err: out.resultErr(runErr)}
}

// resultErr is Result.Err: what the body returned, joined with every misuse the run recorded.
func (r *Run) resultErr(runErr error) error {
	return joinErrors(runErr, r.allMisuse())
}

func (r *Run) anyFailed() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tasks {
		if t.node.Rec.State() == failed {
			return true
		}
	}
	return false
}

// rendererError reports a renderer failure as a genuine Run failure; nil stays nil.
func rendererError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", errRenderer, err)
}

func (r *Run) conclusion() Conclusion {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.rec.Conclusion(); ok {
		return asEvoConclusion(c)
	}
	snap := r.snapshotLocked()
	c := record.InferConclusion(snap)
	r.explainCancellationLocked(&c)
	return asEvoConclusion(c)
}

const cancelCauseUser = "by user"

const cancelCauseClose = "run closed"

// explainCancellationLocked names what interrupted a cancelled run as its Explanation.
func (r *Run) explainCancellationLocked(c *record.Conclusion) {
	if c.Outcome != record.OutcomeCancelled {
		return
	}
	if cause := r.graph.CancelCause(); cause != "" {
		c.Explanation = cause
	}
}
