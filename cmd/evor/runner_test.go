package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func validConfig() Config {
	return Config{
		Title:     "evor-test",
		Mode:      modeStream,
		Heartbeat: "12s",
		Stages: []Stage{{
			Name: "one",
			Work: Command{Command: []string{"true"}},
		}},
	}
}

func TestCreateEvidenceDir_ExistingDirectory_Rejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, _, err := createEvidenceDir(dir)
	if err == nil {
		t.Fatal("expected error for existing evidence directory")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %v, want already exists", err)
	}
}

func TestNewRunner_HeartbeatBelowOneSecond_Rejected(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Heartbeat = "500ms"
	_, err := newRunner(t.TempDir(), "evor.json", cfg, filepath.Join(t.TempDir(), "ev"))
	if err == nil {
		t.Fatal("expected error for heartbeat below 1s")
	}
	if !strings.Contains(err.Error(), "1s") {
		t.Fatalf("error = %v, want heartbeat minimum 1s", err)
	}
}

func TestNewRunner_UnresolvedAutoMode_Rejected(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Mode = modeAuto
	_, err := newRunner(t.TempDir(), "evor.json", cfg, filepath.Join(t.TempDir(), "ev"))
	if err == nil {
		t.Fatal("expected error for unresolved auto mode")
	}
}

func TestNewRunner_TTYMode_DisablesRawPassthrough(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Mode = modeTTY
	r, err := newRunner(t.TempDir(), "evor.json", cfg, filepath.Join(t.TempDir(), "ev"))
	if err != nil {
		t.Fatal(err)
	}
	if r.stream {
		t.Fatal("tty mode must disable raw child passthrough")
	}
	if streamMode(r.cfg.Mode) {
		t.Fatal("tty mode must set evo Plain false")
	}
}

func TestNewRunner_StreamMode_EnablesRawPassthrough(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Mode = modeStream
	r, err := newRunner(t.TempDir(), "evor.json", cfg, filepath.Join(t.TempDir(), "ev"))
	if err != nil {
		t.Fatal(err)
	}
	if !r.stream {
		t.Fatal("stream mode must enable raw child passthrough")
	}
	if !streamMode(r.cfg.Mode) {
		t.Fatal("stream mode must set evo Plain true")
	}
}

func TestNewRunner_WritesResolvedModeToManifest(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Mode = modeTTY
	evidence := filepath.Join(t.TempDir(), "ev")
	r, err := newRunner(t.TempDir(), "evor.json", cfg, evidence)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(r.evidence, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got manifest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Mode != modeTTY {
		t.Fatalf("manifest mode = %q, want %q", got.Mode, modeTTY)
	}
}

func TestSetup_MissingRoot_ReturnsError(t *testing.T) {
	t.Parallel()
	_, _, _, _, err := setup(filepath.Join(t.TempDir(), "missing"), "evor.json", false, "")
	if err == nil {
		t.Fatal("expected error for missing project root")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("error = %v, want not a directory", err)
	}
}

func TestSetup_Smoke_ResolvesPresentation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_, r, title, _, err := setup(root, "evor.json", true, filepath.Join(t.TempDir(), "ev"))
	if err != nil {
		t.Fatal(err)
	}
	if title != "evor smoke" {
		t.Fatalf("title = %q, want %q", title, "evor smoke")
	}
	if r == nil {
		t.Fatal("runner is nil")
	}
	if r.cfg.Mode != modeTTY && r.cfg.Mode != modeStream {
		t.Fatalf("mode = %q, want resolved %q or %q", r.cfg.Mode, modeTTY, modeStream)
	}
}

func TestPublishEvidence_Success_WritesPassedManifestAndBundle(t *testing.T) {
	cfg := validConfig()
	r, err := newRunner(t.TempDir(), "evor.json", cfg, filepath.Join(t.TempDir(), "ev"))
	if err != nil {
		t.Fatal(err)
	}
	evo.Init(evo.Config{Title: "evor-test", Plain: true, Stdout: io.Discard, Stderr: io.Discard})

	if err := r.publishEvidence(nil); err != nil {
		t.Fatal(err)
	}

	got := readManifest(t, r.evidence)
	if got.Result != "passed" {
		t.Fatalf("result = %q, want passed", got.Result)
	}
	if _, err := os.Stat(r.bundle); err != nil {
		t.Fatalf("bundle missing: %v", err)
	}
}

func TestPublishEvidence_RunError_WritesFailedManifest(t *testing.T) {
	cfg := validConfig()
	r, err := newRunner(t.TempDir(), "evor.json", cfg, filepath.Join(t.TempDir(), "ev"))
	if err != nil {
		t.Fatal(err)
	}
	evo.Init(evo.Config{Title: "evor-test", Plain: true, Stdout: io.Discard, Stderr: io.Discard})

	if err := r.publishEvidence(errors.New("stage failed")); err != nil {
		t.Fatal(err)
	}

	got := readManifest(t, r.evidence)
	if got.Result != "failed" {
		t.Fatalf("result = %q, want failed", got.Result)
	}
}

func readManifest(t *testing.T, evidence string) manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(evidence, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got manifest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	return got
}
