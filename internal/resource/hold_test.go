package resource

import (
	"context"
	"sync"
)

// contentionObservers maps a test Registry to the hook its Hold calls
// report contention to. Production code reports contention per request
// (Request.OnContended); tests that hold pre-resolved Claims observe a
// whole Registry instead.
var contentionObservers sync.Map

// newObservedRegistry is a Registry whose Hold calls report every claim
// made to wait to observe.
func newObservedRegistry(observe func(Claim)) *Registry {
	r := NewRegistry()
	contentionObservers.Store(r, observe)
	return r
}

// Hold holds an already-resolved Claim for fn, the test-side counterpart of
// HoldResource.
func (r *Registry) Hold(ctx context.Context, c Claim, fn func(context.Context) error) error {
	observe, _ := contentionObservers.Load(r)
	hook, _ := observe.(func(Claim))
	return r.hold(ctx, c, hook, fn)
}
