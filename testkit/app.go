package testkit

import "github.com/zachbornheimer/evident-output/internal/fingerprint"

// WithFakeApp runs fn while evo.App() observes identity as the running
// executable's bytes, restoring the previous observation afterward.
// Test-only: simulates an application-fingerprint change without replacing
// the process binary mid-test.
func WithFakeApp(identity string, fn func()) {
	fingerprint.WithFakeApp(identity, fn)
}
