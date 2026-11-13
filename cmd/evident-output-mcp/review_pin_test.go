package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

func TestWriteReviewResult_UpdateNeededFalseWhenServerNewer(t *testing.T) {
	prev := reviewIdentity
	reviewIdentity = func() Identity { return Identity{Version: "9.9.9"} }
	defer func() { reviewIdentity = prev }()

	msg := captureRPC(t, func() {
		writeReviewResult(1, review.Result{DesiredVersion: "v0.4.6", ModuleVersion: "v0.4.6"})
	})
	sc := structured(t, msg)
	if sc["update_needed"] != false {
		t.Fatalf("update_needed=%v want false", sc["update_needed"])
	}
	if sc["mcp_version"] != "9.9.9" {
		t.Fatalf("mcp_version=%v", sc["mcp_version"])
	}
	if sc["desired_version"] != "v0.4.6" {
		t.Fatalf("desired_version=%v", sc["desired_version"])
	}
	if sc["module_version"] != "v0.4.6" {
		t.Fatalf("module_version=%v", sc["module_version"])
	}
}

func TestWriteReviewResult_UpdateNeededTrueOnReplaceMismatch(t *testing.T) {
	prev := reviewIdentity
	reviewIdentity = func() Identity { return Identity{Version: "0.4.6", SourceDir: "/not/that"} }
	defer func() { reviewIdentity = prev }()

	msg := captureRPC(t, func() {
		writeReviewResult(1, review.Result{
			DesiredVersion:  "v0.4.6",
			ModuleVersion:   "v0.4.6",
			ReplacePath:     "/src/evo",
			Findings:        []review.Finding{{RuleID: "API-032", Message: "evo.New"}},
			RecheckRequired: true,
		})
	})
	sc := structured(t, msg)
	if sc["update_needed"] != true {
		t.Fatalf("update_needed=%v want true", sc["update_needed"])
	}
	next, _ := sc["next_action"].(string)
	if !strings.Contains(next, "evident_output_update") || !strings.Contains(next, "restart") {
		t.Fatalf("next_action must instruct update then restart first, got %q", next)
	}
	if !strings.Contains(next, "evident_output_review") {
		t.Fatalf("next_action must keep the review-loop sentence when findings remain, got %q", next)
	}
	idxUpdate := strings.Index(next, "evident_output_update")
	idxLoop := strings.Index(next, "evident_output_review")
	if idxUpdate < 0 || idxLoop < 0 || idxUpdate > idxLoop {
		t.Fatalf("update instruction must precede review-loop sentence: %q", next)
	}
}

func TestWriteReviewResult_DevVersionAloneDoesNotNeedUpdate(t *testing.T) {
	prev := reviewIdentity
	reviewIdentity = func() Identity { return Identity{Version: "dev"} }
	defer func() { reviewIdentity = prev }()

	msg := captureRPC(t, func() {
		writeReviewResult(1, review.Result{DesiredVersion: "v0.4.6", ModuleVersion: "v0.4.6"})
	})
	sc := structured(t, msg)
	if sc["update_needed"] != false {
		t.Fatalf("update_needed=%v want false when Version is only dev", sc["update_needed"])
	}
}

func TestReview_DesiredVersionV029DoesNotFlagNew(t *testing.T) {
	src := `package main
import evo "github.com/zachbornheimer/evident-output"
func main() {
  _ = evo.New(evo.Config{Title: "t"})
}
`
	args := map[string]any{"source": src, "file": "main.go", "desired_version": "v0.2.9"}
	msg := captureRPC(t, func() {
		handleToolCall(1, map[string]any{
			"params": map[string]any{
				"name":      "evident_output_review",
				"arguments": args,
			},
		})
	})
	raw, _ := json.Marshal(msg)
	if strings.Contains(string(raw), "API-032") {
		t.Fatalf("evo.New must not be API-032 at pre-0.4 desired_version: %s", raw)
	}
	current := captureRPC(t, func() {
		handleToolCall(2, map[string]any{
			"params": map[string]any{
				"name":      "evident_output_review",
				"arguments": map[string]any{"source": src, "file": "main.go"},
			},
		})
	})
	curRaw, _ := json.Marshal(current)
	if !strings.Contains(string(curRaw), "API-032") {
		t.Fatalf("evo.New must be API-032 at the current dialect: %s", curRaw)
	}
}

func TestReviewDirectory_FillsPinFields(t *testing.T) {
	dir := t.TempDir()
	mod := "module app\n\nrequire github.com/zachbornheimer/evident-output v0.4.6\nreplace github.com/zachbornheimer/evident-output => ./evo\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "evo"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := `package a
import evo "github.com/zachbornheimer/evident-output"
func f() { _ = evo.Init(evo.Config{}) }
`
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := review.GoDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.ModuleVersion != "v0.4.6" {
		t.Fatalf("ModuleVersion=%q", res.ModuleVersion)
	}
	if res.DesiredVersion != "v0.4.6" {
		t.Fatalf("DesiredVersion=%q", res.DesiredVersion)
	}
	wantReplace := filepath.Join(dir, "evo")
	if res.ReplacePath != wantReplace {
		t.Fatalf("ReplacePath=%q want %q", res.ReplacePath, wantReplace)
	}
}

func captureRPC(t *testing.T, fn func()) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	outMu.Lock()
	prevW, prevM := outW, outMode
	outW = &buf
	outMode = frameNDJSON
	outMu.Unlock()
	defer func() {
		outMu.Lock()
		outW = prevW
		outMode = prevM
		outMu.Unlock()
	}()
	fn()
	var msg map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &msg); err != nil {
		t.Fatalf("rpc json: %v (%q)", err, buf.String())
	}
	return msg
}

func structured(t *testing.T, msg map[string]any) map[string]any {
	t.Helper()
	result, _ := msg["result"].(map[string]any)
	sc, _ := result["structuredContent"].(map[string]any)
	if sc == nil {
		t.Fatalf("missing structuredContent: %v", msg)
	}
	return sc
}
