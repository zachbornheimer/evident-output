// Package misuse owns the codes, remedy text and recording of API misuse.
//
// It imports only the standard library, internal/record and the facades.
// Scheduling, freshness, change, engine and project stay out of it: they
// report misuse here, and it records the first one. A strict recorder
// panics after recording; callers must never hold graph.mu when they
// report, because that panic would strand the lock.
package misuse
