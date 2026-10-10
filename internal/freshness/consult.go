// This file owns the satisfied-or-stale judgment of one tracked operation
// against the manifest's prior record of it.

package freshness

import "context"

// Reasons a Consultation reports for its verdict (spec §38: events must
// distinguish Basis drift from tracked output drift from no prior record at
// all).
const (
	ReasonNoPriorRecord      = "no_prior_record"
	ReasonBasisDrift         = "basis_drift"
	ReasonDefinitionDrift    = "definition_changed"
	ReasonTrackedOutputDrift = "tracked_output_drift"
	ReasonCurrent            = "current"
	// ReasonNoOutputsDeclared is Exec's own "not current" reason (spec §8.4:
	// "No Outputs → always run"): there is no prior-record comparison to make
	// at all, since Exec has nothing observable to prove a prior run's effect
	// still holds.
	ReasonNoOutputsDeclared = "no_outputs_declared"
)

// Consultation is what the manifest says about one operation: whether it is
// still current, why, and the prior record to carry forward on a current hit.
type Consultation struct {
	Current               bool
	Reason                string
	Prior                 OperationRecord
	DefinitionFingerprint string
}

// Consult reports whether the previously committed record for the Task's
// ordinal-th operation in key, if the store has one, still matches this File
// call: every Basis digest, the operation definition itself, and the tracked
// output's current on-disk digest (spec §8.2/§11.4/§11.5). A prior record's
// absence, a Basis/definition mismatch, or output drift (edited outside Evo)
// are all "not current", never an error on their own; Reason names which.
//
// Basis is checked ahead of the combined definition fingerprint (which
// itself already hashes Basis in) so a Basis-only change reports
// ReasonBasisDrift rather than being folded into the more generic
// definition mismatch.
func (c FileCall) Consult(ctx context.Context, store *ManifestStore, key string, ordinal int) (Consultation, error) {
	definition := c.DefinitionFingerprint()
	prior, hasPrior := store.Operation(key, ordinal)
	verdict := Consultation{Prior: prior, DefinitionFingerprint: definition}
	if !hasPrior || len(prior.Outputs) != 1 {
		verdict.Reason = ReasonNoPriorRecord
		return verdict, nil
	}
	if !BasisRecordsEqual(prior.Basis, c.Basis) {
		verdict.Reason = ReasonBasisDrift
		return verdict, nil
	}
	if prior.DefinitionFingerprint != definition {
		verdict.Reason = ReasonDefinitionDrift
		return verdict, nil
	}
	moved, err := outputMoved(ctx, prior.Outputs[0].Path, prior.Outputs[0].Digest)
	if err != nil {
		return Consultation{}, err
	}
	if moved {
		verdict.Reason = ReasonTrackedOutputDrift
		return verdict, nil
	}
	verdict.Current, verdict.Reason = true, ReasonCurrent
	return verdict, nil
}

// Record is the operation record a freshly committed File leaves: its
// definition, the Basis observed before the commit (a Basis that changes
// during the write is drift the next Run must see, not state to paper over),
// and the managed path's content identity as it is now.
func (c FileCall) Record(ctx context.Context) (OperationRecord, error) {
	identity, err := PathOutputDigest(ctx, c.Path)
	if err != nil {
		return OperationRecord{}, err
	}
	return OperationRecord{
		Kind:                  OperationKindFile,
		DefinitionFingerprint: c.DefinitionFingerprint(),
		Basis:                 c.Basis,
		Outputs:               []OutputRecord{{Kind: OperationKindFile, Path: c.Path, Digest: identity}},
	}, nil
}

// ExecDisposition is how an Exec call proceeds once its freshness is known.
type ExecDisposition int

const (
	// ExecRuns: the manifest cannot prove the call current; spawn it.
	ExecRuns ExecDisposition = iota
	// ExecIsCurrent: the manifest proves the call current; spawn nothing and
	// carry the prior record forward.
	ExecIsCurrent
	// ExecIsPlanned: the call is stale but this is a dry run; spawn nothing
	// and plan the Effect.
	ExecIsPlanned
)

// ExecEvaluation is an Exec call's freshness verdict. When it runs,
// DefinitionFingerprint and Basis are what the post-spawn success record
// commits.
type ExecEvaluation struct {
	Disposition           ExecDisposition
	Reason                string
	Prior                 OperationRecord
	DefinitionFingerprint string
	Basis                 []BasisRecord
}

// Spawns reports whether the call must actually start the child.
func (e ExecEvaluation) Spawns() bool { return e.Disposition == ExecRuns }

// Evaluate judges the Exec call against the manifest (spec
// §11.4/§11.5/§8.4): current when the definition, Basis and every declared
// output's on-disk identity still match the prior record, otherwise stale.
// A stale call is only planned in a dry run.
func (c ExecCall) Evaluate(ctx context.Context, store *ManifestStore, key string, ordinal int, dryRun bool) (ExecEvaluation, error) {
	executableIdentity, err := PathOutputDigest(ctx, c.ExecutablePath)
	if err != nil {
		return ExecEvaluation{}, err
	}
	definition := c.definitionFingerprint(executableIdentity)
	prior, hasPrior := store.Operation(key, ordinal)
	reason, err := c.reason(ctx, prior, hasPrior, definition)
	if err != nil {
		return ExecEvaluation{}, err
	}
	evaluation := ExecEvaluation{Reason: reason, Prior: prior, DefinitionFingerprint: definition, Basis: c.Basis}
	switch {
	case reason == ReasonCurrent:
		evaluation.Disposition = ExecIsCurrent
	case dryRun:
		evaluation.Disposition = ExecIsPlanned
	}
	return evaluation, nil
}

// reason is the Exec's verdict, mirroring FileCall.Consult's categories
// (spec §38) so Exec's operation.started/skipped_current payloads carry the
// same "reason" vocabulary File's do.
func (c ExecCall) reason(ctx context.Context, prior OperationRecord, hasPrior bool, definition string) (string, error) {
	if len(c.Outputs) == 0 {
		return ReasonNoOutputsDeclared, nil
	}
	if !hasPrior || len(prior.Outputs) != len(c.Outputs) {
		return ReasonNoPriorRecord, nil
	}
	if !BasisRecordsEqual(prior.Basis, c.Basis) {
		return ReasonBasisDrift, nil
	}
	if prior.DefinitionFingerprint != definition {
		return ReasonDefinitionDrift, nil
	}
	for i, out := range c.Outputs {
		if prior.Outputs[i].Path != out {
			return ReasonTrackedOutputDrift, nil
		}
		moved, err := outputMoved(ctx, out, prior.Outputs[i].Digest)
		if err != nil {
			return "", err
		}
		if moved {
			return ReasonTrackedOutputDrift, nil
		}
	}
	return ReasonCurrent, nil
}

// RecordedOutputsHold reports whether every Output recorded by the Task's
// last successful Run still has the content identity recorded then. An
// Output that can no longer be observed does not hold; only a done ctx is an
// error.
func RecordedOutputsHold(ctx context.Context, operations []OperationRecord) (bool, error) {
	for _, op := range operations {
		for _, output := range op.Outputs {
			moved, err := outputMoved(ctx, output.Path, output.Digest)
			if err != nil || moved {
				return false, ctx.Err()
			}
		}
	}
	return true, nil
}

// outputMoved reports whether the content at path no longer has the content
// identity recorded for it. The identity is a public content digest, never a
// secret, so an ordinary comparison is right.
func outputMoved(ctx context.Context, path, recordedIdentity string) (bool, error) {
	observed, err := PathOutputDigest(ctx, path)
	if err != nil {
		return false, err
	}
	return observed != recordedIdentity, nil
}
