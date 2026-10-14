package evo

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/misuse"
)

func quietOutput(cfg Config) *Run {
	cfg.plain, cfg.color = true, colorNever
	cfg.stdout, cfg.stderr = io.Discard, io.Discard
	return initRun(cfg)
}

// TestRejectedDeclaration_DefineRecordsMisuse: Define on a rejected handle
// never runs fn and is misuse, not a silent drop.
func TestRejectedDeclaration_DefineRecordsMisuse(t *testing.T) {
	out := quietOutput(Config{})
	t.Cleanup(func() { _ = out.close() })
	out.Task("x")
	ran := false
	dup := out.Task("x")
	out.mu.Lock()
	out.misuse = misuse.New(false)
	out.mu.Unlock()
	dup.Define(func(context.Context) error { ran = true; return nil })
	_ = out.finish()
	if ran {
		t.Fatal("Define on a rejected handle ran its callback")
	}
	if err := misuseOf(out); !errors.Is(err, errDuplicateSiblingName) {
		t.Fatalf("Define on a rejected handle must record misuse, got %v", err)
	}
}
