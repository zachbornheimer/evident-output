//go:build v06acceptance

// Package file_test holds increment 2's v0.6/1.0 public API compile
// contract: File/FileSpec and Config.AppID/Config.StateDir (spec §8,
// §11.3). Split out of ../pending/pending_test.go, which still carries the
// remaining increment-3 ExecSpec contract and stays red-by-compile until
// evo.Exec exists.
package file_test

import (
	"context"
	"io/fs"
	"os"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

var _ = func() func(context.Context, evo.FileSpec) error { return evo.File }

func TestV06FileAPITypesCompile(t *testing.T) {
	var _ evo.Config = evo.Config{
		Title:    "contract",
		AppID:    "example",
		StateDir: t.TempDir(),
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		Isolated: true,
	}
	var _ evo.FileSpec = evo.FileSpec{
		Path:     "out",
		Contents: []byte{},
		Mode:     fs.FileMode(0o644),
		Basis:    []evo.Fingerprint{evo.FSPath("in")},
	}
}
