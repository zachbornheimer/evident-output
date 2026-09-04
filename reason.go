package evo

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/sanitize"
)

// TaxonomyReason names why a task skipped or kept an item. It is the opaque
// handle returned by evo.Reason — duplicate strings merge into one taxonomy
// bucket so a caller can construct one inline at every call site
// (evo.Reason("dirty")) without hand-tracking identity, or lift it to a
// package var once it repeats.
type TaxonomyReason struct {
	name    string
	forSkip bool
	onTask  string // empty means usable from any task
}

// Name returns the reason's display label.
func (r TaxonomyReason) Name() string { return r.name }

// ReasonOption constrains how a Reason may be used; a violated constraint is
// recorded as misuse (Strict panics; production still counts the record).
type ReasonOption interface {
	apply(*TaxonomyReason)
}

type reasonOptionFunc func(*TaxonomyReason)

func (f reasonOptionFunc) apply(r *TaxonomyReason) { f(r) }

// ForSkip restricts a reason to TaskHandle.Skipped — recording it via Kept is misuse.
func ForSkip() ReasonOption {
	return reasonOptionFunc(func(r *TaxonomyReason) { r.forSkip = true })
}

// OnTask restricts a reason to the named task — recording it from a
// different task is misuse.
func OnTask(taskName string) ReasonOption {
	return reasonOptionFunc(func(r *TaxonomyReason) { r.onTask = taskName })
}

// formatReasonName splits args into printf format arguments and
// ReasonOption values, mirroring formatEntityName's mixed-args extraction
// (C6: name is a printf format when fmt args are present; evo.ForSkip()/
// evo.OnTask(...) may be mixed into args in any position).
func formatReasonName(name string, args []any) (string, []ReasonOption) {
	if len(args) == 0 {
		return name, nil
	}
	var opts []ReasonOption
	var fmtArgs []any
	for _, a := range args {
		if opt, ok := a.(ReasonOption); ok {
			opts = append(opts, opt)
			continue
		}
		fmtArgs = append(fmtArgs, a)
	}
	if len(fmtArgs) == 0 {
		return name, opts
	}
	return fmt.Sprintf(name, fmtArgs...), opts
}

// reasonGetOrCreate returns the Reason previously registered under name on
// this instance, or registers a new one — the identity backing evo.Reason so
// repeated calls (inline or lifted to a var) merge into one taxonomy bucket
// instead of drifting into differently-configured duplicates.
func (o *Output) reasonGetOrCreate(name string, opts ...ReasonOption) TaxonomyReason {
	name = sanitize.Text(name)
	o.mu.Lock()
	defer o.mu.Unlock()
	if r, ok := o.namedReasons[name]; ok {
		return r
	}
	r := TaxonomyReason{name: name}
	for _, opt := range opts {
		if opt != nil {
			opt.apply(&r)
		}
	}
	if o.namedReasons == nil {
		o.namedReasons = make(map[string]TaxonomyReason)
	}
	o.namedReasons[name] = r
	return r
}
