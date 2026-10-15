// This file owns the fault injection that lets a test intervene at each step
// of a commit.

package change

import "sync/atomic"

// PublishStep is a point in a commit where a test may intervene, in the order a
// tree commit reaches them.
type PublishStep int

const (
	// PublishStaged: the content is staged and the lock not yet requested.
	PublishStaged PublishStep = iota + 1
	// PublishLocked: the lock is held; nothing is revalidated or renamed.
	PublishLocked
	// PublishAside: no-exchange path only; the original is moved aside and
	// the staged tree is not yet at dest, so dest is absent.
	PublishAside
	// PublishSwapped: the staged content is at dest and any original beside
	// it; nothing is verified or cleaned up.
	PublishSwapped
	// PublishReleasing: the commit's work under the lock is done; the lock
	// is about to be released.
	PublishReleasing
)

// PublishFaults is what a test injects into every commit in this process. The
// zero value injects nothing.
type PublishFaults struct {
	// At runs at each Step a commit of dest reaches. Exiting the process
	// here is a crash at that step.
	At func(step PublishStep, dest string)
	// NoExchange makes a tree commit take the move-aside path of platforms
	// with no atomic exchange.
	NoExchange bool
}

var injected atomic.Pointer[PublishFaults]

// InjectPublishFaults installs f for every later commit and returns the function
// that removes it. Tests only.
func InjectPublishFaults(f PublishFaults) (restore func()) {
	injected.Store(&f)
	return func() { injected.Store(nil) }
}

// reach runs the injected fault, if any, for dest at step.
func reach(step PublishStep, dest string) {
	if f := injected.Load(); f != nil && f.At != nil {
		f.At(step, dest)
	}
}

// exchangeAllowed reports whether a tree commit may try an atomic exchange.
func exchangeAllowed() bool {
	f := injected.Load()
	return f == nil || !f.NoExchange
}
