package evo_test

import (
	"context"
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleFingerprint observes one Basis input's current content identity —
// FSPath, Value, and App are its three constructors; a custom
// implementation may observe anything else Basis needs to track.
func ExampleFingerprint() {
	fp := evo.Value("go-version", "1.23")
	v, err := fp.Fingerprint(context.Background())
	fmt.Println(err == nil, v.Kind != "")
	// Output:
	// true true
}

// ExampleFingerprintValue is one Fingerprint's observed identity: a stable
// machine Kind, a stable non-secret Key, and a SHA-256 Digest — only the
// digest and name are ever persisted, never the raw value.
func ExampleFingerprintValue() {
	v, err := evo.Value("go-version", "1.23").Fingerprint(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(v.Key)
	// Output:
	// go-version
}

// ExampleFSPath fingerprints the filesystem content at path: a regular
// file's type-marked byte digest, a directory's Merkle digest over sorted
// entries, a symlink's target text, or a stable "missing" digest.
func ExampleFSPath() {
	f, err := os.CreateTemp("", "evo-example-fspath")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.Remove(f.Name()) }()
	_ = f.Close()

	fp := evo.FSPath(f.Name())
	v, err := fp.Fingerprint(context.Background())
	fmt.Println(err == nil, v.Key == f.Name())
	// Output:
	// true true
}

// ExampleValue fingerprints a caller-supplied scalar under a stable, safe
// name — only the digest and name are ever persisted, never the raw value.
func ExampleValue() {
	fp := evo.Value("feature-flag", true)
	v, err := fp.Fingerprint(context.Background())
	fmt.Println(err == nil, v.Key)
	// Output:
	// true feature-flag
}

// ExampleApp fingerprints the running application itself — include it in
// an operation's Basis only when the application's own implementation is a
// semantic input to that operation's result.
func ExampleApp() {
	fp := evo.App()
	v, err := fp.Fingerprint(context.Background())
	fmt.Println(err == nil, v.Kind != "")
	// Output:
	// true true
}
