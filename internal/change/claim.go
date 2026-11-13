// This file declares what a claim is: the mode it holds a resource in, the
// request that names it, and the misuse of holding two at once.

package change

import "errors"

// ClaimMode is how a claim touches its resource.
type ClaimMode uint8

const (
	// ClaimRead observes the resource; any number of overlapping reads share it.
	ClaimRead ClaimMode = iota
	// ClaimWrite changes the resource; it excludes every overlapping claim.
	ClaimWrite
)

// String renders the mode as "read" or "write".
func (m ClaimMode) String() string {
	if m == ClaimWrite {
		return "write"
	}
	return "read"
}

// Claim is one request for a resolved ResourceKey in a ClaimMode.
type Claim struct {
	Key  ResourceKey
	Mode ClaimMode
}

// String renders the claim as "<mode> <key>", e.g. "write fs:/repo".
func (c Claim) String() string { return c.Mode.String() + " " + c.Key.String() }

// conflicts reports whether c and o may not be held at the same time:
// their keys overlap and at least one of them writes.
func (c Claim) conflicts(o Claim) bool {
	return (c.Mode == ClaimWrite || o.Mode == ClaimWrite) && c.Key.overlaps(o.Key)
}

// ErrNestedResourceAcquisition is returned, without waiting, when a context that already
// holds a resource asks for another one. Holding at most one resource at a
// time is what rules deadlock out, so a second acquisition is misuse even
// when it would not conflict.
var ErrNestedResourceAcquisition = errors.New("evo: nested resource acquisition")

// ClaimRequest is an unresolved claim: a Resource, the workspace a relative
// filesystem path resolves against, and the access Mode.
type ClaimRequest struct {
	Resource  Resource
	Workspace string
	Mode      ClaimMode
	// OnContended, when non-nil, runs once, outside any registry lock, and
	// only if the claim has to wait. It lets one process-wide ClaimRegistry
	// report contention to whichever caller is actually waiting.
	OnContended func(Claim)
}
