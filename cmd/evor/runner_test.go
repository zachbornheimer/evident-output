package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validConfig() ProjectConfig {
	return ProjectConfig{
		Title:        "evor-test",
		Mode:         modeStream,
		Heartbeat:    "12s",
		AgentTimeout: "90m",
		Verify: []Command{{
			Name:    "true",
			Command: []string{"true"},
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

func TestCreateEvidenceDir_CreatesMissingDir(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "ev")
	got, bundle, err := createEvidenceDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(got)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", got)
	}
	if bundle != got+".zip" {
		t.Fatalf("bundle = %q, want %q", bundle, got+".zip")
	}
}

func TestCreateEvidenceDir_EmptyRequested_UsesUniqueMkdirTemp(t *testing.T) {
	t.Parallel()
	a, aBundle, err := createEvidenceDir("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(a) })
	b, bBundle, err := createEvidenceDir("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(b) })
	if a == b {
		t.Fatal("omitted evidence dir must be unique per call")
	}
	if !strings.HasPrefix(filepath.Base(a), "evor-") || !strings.HasPrefix(filepath.Base(b), "evor-") {
		t.Fatalf("dirs = %q, %q, want MkdirTemp prefix evor-", a, b)
	}
	if aBundle != a+".zip" || bBundle != b+".zip" {
		t.Fatalf("bundles = %q, %q, want dir+\".zip\"", aBundle, bBundle)
	}
}

func TestCreateEvidenceDir_ExistingBundle_Rejected(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "ev")
	bundle := dir + ".zip"
	if err := os.WriteFile(bundle, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := createEvidenceDir(dir)
	if err == nil {
		t.Fatal("expected error for existing results bundle")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %v, want already exists", err)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatalf("newly created dir should be removed when bundle exists, stat = %v", statErr)
	}
}

func TestNewRunner_TTYMode_DisablesRawPassthrough(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	r, err := newRunner(t.TempDir(), "evor.json", "prompt.md", "grok", modeTTY, cfg, filepath.Join(t.TempDir(), "ev"))
	if err != nil {
		t.Fatal(err)
	}
	if r.mode == "stream" {
		t.Fatal("tty mode must disable raw child passthrough")
	}
	if streamMode(r.mode) {
		t.Fatal("tty mode must set evo Plain false")
	}
}

func TestNewRunner_StreamMode_EnablesRawPassthrough(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	r, err := newRunner(t.TempDir(), "evor.json", "prompt.md", "grok", modeStream, cfg, filepath.Join(t.TempDir(), "ev"))
	if err != nil {
		t.Fatal(err)
	}
	if r.mode != "stream" {
		t.Fatal("stream mode must enable raw child passthrough")
	}
	if !streamMode(r.mode) {
		t.Fatal("stream mode must set evo Plain true")
	}
}

func TestNewRunner_WritesResolvedModeToManifest(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	evidence := filepath.Join(t.TempDir(), "ev")
	r, err := newRunner(t.TempDir(), "evor.json", "prompt.md", "grok", modeTTY, cfg, evidence)
	if err != nil {
		t.Fatal(err)
	}
	got := readManifest(t, r.evidence)
	if got.Mode != modeTTY {
		t.Fatalf("manifest mode = %q, want %q", got.Mode, modeTTY)
	}
}

func TestAbsoluteDir_MissingRoot_ReturnsError(t *testing.T) {
	t.Parallel()
	_, err := absoluteDir(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("expected error for missing project root")
	}
}

func TestAbsoluteDir_FileNotDirectory_ReturnsError(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := absoluteDir(file)
	if err == nil {
		t.Fatal("expected error for non-directory")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("error = %v, want not a directory", err)
	}
}

func TestSmokeProjectConfig_ResolvesAutoMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cfg := ProjectConfig{
		Title:        "evor smoke",
		Mode:         modeAuto,
		Attempts:     1,
		Heartbeat:    "1s",
		AgentTimeout: "10s",
		Verify: []Command{{
			Name:    "synthetic verifier",
			Command: []string{"sh", "-c", "echo verifier-ok"},
			Timeout: "5s",
		}},
	}
	if cfg.Title != "evor smoke" {
		t.Fatalf("title = %q, want %q", cfg.Title, "evor smoke")
	}
	mode, err := resolveMode(modeAuto, false)
	if err != nil {
		t.Fatal(err)
	}
	if mode != modeStream {
		t.Fatalf("auto + non-TTY mode = %q, want %q", mode, modeStream)
	}
	r, err := newRunner(root, "", "", "", mode, cfg, filepath.Join(t.TempDir(), "ev"))
	if err != nil {
		t.Fatal(err)
	}
	if r.mode != modeStream {
		t.Fatalf("runner mode = %q, want %q", r.mode, modeStream)
	}
}

func TestFinish_Zero_WritesPassedManifestAndBundle(t *testing.T) {
	silenceStdout(t)
	cfg := validConfig()
	r, err := newRunner(t.TempDir(), "evor.json", "prompt.md", "grok", modeStream, cfg, filepath.Join(t.TempDir(), "ev"))
	if err != nil {
		t.Fatal(err)
	}

	if code := r.finish(0); code != 0 {
		t.Fatalf("finish(0) = %d, want 0", code)
	}

	got := readManifest(t, r.evidence)
	if got.Result != "passed" {
		t.Fatalf("result = %q, want passed", got.Result)
	}
	result := readResult(t, r.evidence)
	if result.Result != "passed" {
		t.Fatalf("result.json result = %q, want passed", result.Result)
	}
	if result.ExitCode != 0 {
		t.Fatalf("result.json exit_code = %d, want 0", result.ExitCode)
	}
	if _, err := os.Stat(r.bundle); err != nil {
		t.Fatalf("bundle missing: %v", err)
	}
}

func TestFinish_Nonzero_WritesFailedManifest(t *testing.T) {
	silenceStdout(t)
	cfg := validConfig()
	r, err := newRunner(t.TempDir(), "evor.json", "prompt.md", "grok", modeStream, cfg, filepath.Join(t.TempDir(), "ev"))
	if err != nil {
		t.Fatal(err)
	}
	r.setFailure(errors.New("stage failed"))

	if code := r.finish(1); code != 1 {
		t.Fatalf("finish(1) = %d, want 1", code)
	}

	got := readManifest(t, r.evidence)
	if got.Result != "failed" {
		t.Fatalf("result = %q, want failed", got.Result)
	}
	result := readResult(t, r.evidence)
	if result.Result != "failed" {
		t.Fatalf("result.json result = %q, want failed", result.Result)
	}
	if result.Failure != "stage failed" {
		t.Fatalf("result.json failure = %q, want %q", result.Failure, "stage failed")
	}
	if _, err := os.Stat(r.bundle); err != nil {
		t.Fatalf("bundle missing: %v", err)
	}
}

func silenceStdout(t *testing.T) {
	t.Helper()
	orig := os.Stdout
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = devnull
	t.Cleanup(func() {
		os.Stdout = orig
		_ = devnull.Close()
	})
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

func readResult(t *testing.T, evidence string) resultFile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(evidence, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got resultFile
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	return got
}
