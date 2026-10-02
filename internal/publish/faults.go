package publish

import "sync/atomic"

// Step is a point in a commit where a test may intervene, in the order a
// tree commit reaches them.
type Step int

const (
	// StepStaged: the content is staged and the lock not yet requested.
	StepStaged Step = iota + 1
	// StepLocked: the lock is held; nothing is revalidated or renamed.
	StepLocked
	// StepAside: no-exchange path only; the original is moved aside and
	// the staged tree is not yet at dest, so dest is absent.
	StepAside
	// StepSwapped: the staged content is at dest and any original beside
	// it; nothing is verified or cleaned up.
	StepSwapped
	// StepReleasing: the commit's work under the lock is done; the lock
	// is about to be released.
	StepReleasing
)

// Faults is what a test injects into every commit in this process. The
// zero value injects nothing.
type Faults struct {
	// At runs at each Step a commit of dest reaches. Exiting the process
	// here is a crash at that step.
	At func(step Step, dest string)
	// NoExchange makes a tree commit take the move-aside path of platforms
	// with no atomic exchange.
	NoExchange bool
}

var injected atomic.Pointer[Faults]

// InjectFaults installs f for every later commit and returns the function
// that removes it. Tests only.
func InjectFaults(f Faults) (restore func()) {
	injected.Store(&f)
	return func() { injected.Store(nil) }
}

// reach runs the injected fault, if any, for dest at step.
func reach(step Step, dest string) {
	if f := injected.Load(); f != nil && f.At != nil {
		f.At(step, dest)
	}
}

// exchangeAllowed reports whether a tree commit may try an atomic exchange.
func exchangeAllowed() bool {
	f := injected.Load()
	return f == nil || !f.NoExchange
}
