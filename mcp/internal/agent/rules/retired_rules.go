package rules

import "slices"

// RetiredRule is a review rule whose smell the 2.0 contract cannot express,
// so it has no detector, no catalog entry and no guidance. The ID stays
// here so evident_output_explain can say what became of it instead of
// answering "unknown rule", and so no new rule can reuse the number.
type RetiredRule struct {
	ID        string
	RetiredIn RetiredRelease
	// Why states the 2.0 fact that makes the smell inexpressible.
	Why string
	// Instead states what to write or where the guarantee now lives.
	Instead string
}

// retiredRules is every retired rule, in ID order.
var retiredRules = []RetiredRule{
	{
		ID:        "API-006",
		RetiredIn: RetiredRelease2_0,
		Why:       "it flagged an explicit Start call on a Task handle; Start was removed in 2.0 and no 2.0 type has it, so the call does not compile. A Task begins when its Define callback runs.",
		Instead:   "declare the Task and call Define; use Doing and Progress inside or beside it for the live text.",
	},
	{
		ID:        "API-026",
		RetiredIn: RetiredRelease2_0,
		Why:       "it flagged caller-invented RunAll, Map, Retry, Parallel or Timeout helpers on Evo values; the 2.0 types export only the methods in CONTRACT.md, so none of those names exist and a call to one does not compile.",
		Instead:   "declare one named Task per item under a Group (or a Sequence for ordered work) and order later work with After; keep any retry loop in your own code inside the Define callback.",
	},
	{
		ID:        "API-027",
		RetiredIn: RetiredRelease2_0,
		Why:       "it flagged Done, Fail or Progress called on a Group or Sequence; in 2.0 only a Task has those verbs, so the call is a compile error on a container and a container's state is always derived from its children.",
		Instead:   "resolve the child Task (return from its Define callback), or declare more child Tasks; a Group or Sequence settles from its children and Summary sets only its headline.",
	},
	{
		ID:        "API-028",
		RetiredIn: RetiredRelease2_0,
		Why:       "it flagged a printf-style ending such as Failf, Blockf or Donef whose format string had no directive; those verbs were removed in 1.1, and in 2.0 Fail and Refuse take a literal summary plus options, so there is no format string to leave bare.",
		Instead:   "return evo.Fail(summary, opts...) or evo.Refuse(summary, opts...) from Define; put detail in evo.Detail(text).",
	},
	{
		ID:        "API-029",
		RetiredIn: RetiredRelease2_0,
		Why:       "it flagged DebugWriter used to carry a child process's output into the debug journal; DebugWriter was removed in 2.0 and the journal now takes only structured slog records, so the call does not compile.",
		Instead:   "set cmd.Stdout and cmd.Stderr to task.Writer() inside the Task's Define so the output becomes the live tail and the retained ring; log journal lines through r.SlogHandler().",
	},
	{
		ID:        "API-034",
		RetiredIn: RetiredRelease2_0,
		Why:       "it flagged a statement-form Fail or Block followed by return nil; 2.0 has no Task.Fail or Task.Block, and a dropped evo.Fail, evo.Refuse or evo.Exclude result is the same smell API-040 reports.",
		Instead:   "return the ending from Define: return evo.Fail(summary, evo.Detail(err.Error())), or return evo.Refuse(summary) for a precondition; API-040 flags a dropped ending.",
	},
	{
		ID:        "API-036",
		RetiredIn: RetiredRelease2_0,
		Why:       "it guarded fmt.Sprintf inside the removed printf verbs Failf and Blockf; 2.0 has only evo.Fail(summary, opts...) and evo.Refuse(summary, opts...), so formatting the summary with fmt.Sprintf is the one spelling.",
		Instead:   "build the summary with fmt.Sprintf and return evo.Fail(summary) or evo.Refuse(summary) from Define.",
	},
	{
		ID:        "API-052",
		RetiredIn: RetiredRelease2_0,
		Why:       "it flagged a caller loop that Waits on stored Task handles and hand-counts failures; 2.0 removed Wait, Snapshot and ErrNotStarted from every type, so no program can write that loop.",
		Instead:   "declare each item as a Group child and order later work with After(group), or read a produced value with Computed.Get once the producer is an After predecessor.",
	},
	{
		ID:        "API-059",
		RetiredIn: RetiredRelease2_0,
		Why:       "it flagged a function that kept the FileSet evo.Patch returned but committed a hand-built FileSpec through evo.File, dropping the source Basis; 2.0 removed FileSet, Files and Basis, and Patch(ctx, diff) now commits every file itself and fails with ErrStaleBasis when a source changed, so there is nothing left to discard.",
		Instead:   "call evo.Patch(ctx, diff) and return its error; derive a file from other content with evo.File, not from a Patch result.",
	},
	{
		ID:        "API-061",
		RetiredIn: RetiredRelease2_0,
		Why:       "Record, RecordLabel and RecordName do not exist in 2.0, so no program that compiles can call them; API-032 maps each removed call to its 2.0 replacement.",
		Instead:   "route a real mutation through evo.Effect, information through Task.Fact, and a file write through evo.File or evo.Patch; API-032 spells the rewrite.",
	},
	{
		ID:        "API-062",
		RetiredIn: RetiredRelease2_0,
		Why:       "it flagged a second Skipped or Kept call on one Task; 2.0 removed both were removed in 2.0, and a Task ends exactly once by returning one value, so no program can record two dispositions on one Task.",
		Instead:   "declare one Task per item under the category's Group and return evo.Exclude(reason) from each item's Define; Evo folds the excluded items into one tally.",
	},
	{
		ID:        "API-067",
		RetiredIn: RetiredRelease2_0,
		Why:       "it flagged Into(&x) plumbing a Task result through a caller pointer; Into was removed in 2.0 and no Task type has it, so the call is a compile error.",
		Instead:   "x := evo.Compute(task, fn) returns the value typed; the consumer declares After(x) and reads x.Get() once it is ordered after the producer.",
	},
	{
		ID:        "API-072",
		RetiredIn: RetiredRelease2_0,
		Why:       "it reported evo.Compute used under a pin older than 1.2.0 and emitted a product-gap event; Compute is part of the 2.0 contract and the rule only ever compared a pin to that one release, so no program reviewed at 2.0 can trigger it.",
		Instead:   "review at the 2.0 pin; if a shape the program needs is missing from CONTRACT.md, raise it as a contract issue instead of writing a local substitute.",
	},
	{
		ID:        "DOM-006",
		RetiredIn: RetiredRelease2_0,
		Why:       "it flagged an explicit Start call ahead of Define; Start was removed in 2.0 and no 2.0 type has it, so the call does not compile and a Task never needs it. A Task begins when its Define callback runs.",
		Instead:   "declare the Task and call Define directly, returning evo.Fail, evo.Refuse or evo.Exclude where the work ends that way.",
	},
	{
		ID:        "DOM-014",
		RetiredIn: RetiredRelease2_0,
		Why:       "it flagged Detail(err), an error passed where Detail wants text; Detail now takes a string, so passing an error does not compile.",
		Instead:   "pass evo.Detail(err.Error()) on evo.Fail or evo.Refuse, or return fmt.Errorf(\"...: %w\", err) from Define.",
	},
	{
		ID:        "DOM-016",
		RetiredIn: RetiredRelease2_0,
		Why:       "it flagged an explicit Start call ahead of Doing; Start was removed in 2.0 and no 2.0 type has it, so the call does not compile. Doing is first evidence that a Task is running and needs no activation call.",
		Instead:   "call Doing inside or beside the Task's Define callback; nothing activates a Task but its Define callback running.",
	},
	{
		ID:        "EV-001",
		RetiredIn: RetiredRelease2_0,
		Why:       "it flagged text read back from a retained capture ring and folded into a Fail summary; Task.Writer() returns an io.Writer with no text accessor, so there is no capture text to embed.",
		Instead:   "wire cmd.Stdout and cmd.Stderr to Task.Writer(); Evo keeps the bounded, redacted tail itself. Pass ExecResult.Stderr to evo.Detail only when the failure needs that text as its explanation.",
	},
	{
		ID:        "EVO-EVIDENCE-001",
		RetiredIn: RetiredRelease2_0,
		Why:       "it flagged a mutating callback passed to Task.Evidence; Evidence and its named callbacks were removed in 2.0, so no program can write that call.",
		Instead:   "do the mutation in Define through evo.File, evo.Exec or evo.Effect, and add Verify only when the resulting state can be observed directly; Task.Writer() carries captured output.",
	},
	{
		ID:        "EVO-WIRE-001",
		RetiredIn: RetiredRelease2_0,
		Why:       "it flagged json.Marshal of the internal Snapshot; Snapshot and TasksSnapshot were removed in 2.0, and Result and Conclusion are fixed, documented values, so no internal layout is left to leak.",
		Instead:   "set Config.Output to evo.JSON or evo.JSONL for the versioned run document, and write your own data payload to Run.ResultWriter().",
	},
	{
		ID:        "FP-005",
		RetiredIn: RetiredRelease2_0,
		Why:       "it flagged a Task created already Done, with no Define behind it; Done was removed in 2.0, a Task ends only by returning from its Define callback, and a declared Task paints Running first.",
		Instead:   "submit the work with Define(func(ctx context.Context) error { ... }) and return the ending, so the row shows a Running frame before it settles.",
	},
	{
		ID:        "FP-006",
		RetiredIn: RetiredRelease2_0,
		Why:       "it flagged Doing(...) followed by Done(...) with no Define between them; Done was removed in 2.0, so a row can no longer be narrated and then stamped complete by hand.",
		Instead:   "put the work inside Define(func(ctx context.Context) error { ... }) and call Doing for the item in flight; Evo settles the row from the value Define returns.",
	},
}

// RetiredRules returns every retired rule, in table order.
func RetiredRules() []RetiredRule { return slices.Clone(retiredRules) }

// RetiredRuleByID returns the retired rule with the given ID.
func RetiredRuleByID(id string) (RetiredRule, bool) {
	i := slices.IndexFunc(retiredRules, func(r RetiredRule) bool { return r.ID == id })
	if i < 0 {
		return RetiredRule{}, false
	}
	return retiredRules[i], true
}
