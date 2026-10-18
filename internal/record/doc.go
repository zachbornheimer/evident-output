// Package record owns the truth of one run: the event journal, outcomes, the
// effect ledger, facts, problems and the Conclusion. It imports only the
// standard library and the four facades, so every other internal package
// can depend on it and it depends on none of them.
package record
