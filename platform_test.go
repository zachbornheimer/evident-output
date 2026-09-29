package evo_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/render"

	evo "github.com/zachbornheimer/evident-output"
)

type secretRedactor struct{}

func (secretRedactor) RedactString(s string) string {
	return strings.ReplaceAll(s, "SECRET_TOKEN", "[REDACTED]")
}

func TestCapture_RedactsOnRetention(t *testing.T) {
	var primary bytes.Buffer
	out := evo.Init(evo.Config{
		Title:    "sec",
		Stdout:   &primary,
		Stderr:   &primary,
		Redactor: secretRedactor{},
	})
	task := out.Task("fetch")
	cap := task.Capture()
	_, _ = fmt.Fprintln(cap, "Authorization: Bearer SECRET_TOKEN")
	_ = cap.Close()

	if strings.Contains(cap.Text(), "SECRET_TOKEN") {
		t.Fatalf("token must be redacted in retention: %q", cap.Text())
	}
	if !strings.Contains(cap.Text(), "[REDACTED]") {
		t.Fatalf("expected redaction marker: %q", cap.Text())
	}

	task.Fail("denied", cap.DetailTail())
	_ = out.Finish()
	if strings.Contains(primary.String(), "SECRET_TOKEN") {
		t.Fatalf("token leaked into presentation:\n%s", primary.String())
	}
}

func TestEntityID_StableKeyInSnapshotAndJSON(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "id", Stdout: &buf, Stderr: &buf})
	succeed(out.Task("working tree").Key("gate.working-tree"))
	succeed(out.Task("download base").Key("build.base.download"))
	_ = out.Finish()

	snap := out.Snapshot()
	if len(snap.Tasks) != 2 || snap.Tasks[0].Key != "gate.working-tree" {
		t.Fatalf("first task key: %+v", snap.Tasks)
	}
	if snap.Tasks[1].Key != "build.base.download" {
		t.Fatalf("second task key: %+v", snap.Tasks)
	}

	raw, err := render.EncodeJSON(snap)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	tasks, _ := doc["tasks"].([]any)
	task0, _ := tasks[0].(map[string]any)
	if task0["key"] != "gate.working-tree" {
		t.Fatalf("json first task key: %#v", task0)
	}
	task1, _ := tasks[1].(map[string]any)
	if task1["key"] != "build.base.download" {
		t.Fatalf("json second task key: %#v", task1)
	}
}

func TestEntityID_DuplicateIsMisuse(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "dup", Stdout: &buf, Stderr: &buf})
	succeed(out.Task("a").Key("same"))
	succeed(out.Task("b").Key("same"))
	if !errors.Is(out.Err(), evo.ErrDuplicateKey) {
		t.Fatal("expected ErrDuplicateKey misuse")
	}
}

func TestResultWriter_FormatDataPurity(t *testing.T) {
	var human, result bytes.Buffer
	out := evo.Init(evo.Config{
		Title:  "build",
		Format: evo.FormatData,
		Stdout: &result,
		Stderr: &human,
	})
	succeed(out.Task("compile"))
	succeed(out.Task("link"), "bin/app")
	if _, err := io.WriteString(out.ResultWriter(), `{"artifact":"bin/app"}`+"\n"); err != nil {
		t.Fatal(err)
	}
	_ = out.Finish()

	if strings.Contains(result.String(), "compile") || strings.Contains(result.String(), "link") {
		t.Fatalf("presentation leaked into result stream:\n%s", result.String())
	}
	if !strings.Contains(result.String(), `"artifact":"bin/app"`) {
		t.Fatalf("domain payload missing: %q", result.String())
	}
	if !strings.Contains(human.String(), "compile") {
		t.Fatalf("human presentation missing on stderr:\n%s", human.String())
	}
	if strings.Contains(human.String(), `"artifact"`) {
		t.Fatalf("domain payload leaked into human stream:\n%s", human.String())
	}
}

func TestResultWriter_UnsetIsDiscard(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "h", Stdout: &buf, Stderr: &buf})
	n, err := out.ResultWriter().Write([]byte("should-not-appear"))
	if err != nil {
		t.Fatal(err)
	}
	if n != len("should-not-appear") {
		t.Fatalf("write n=%d", n)
	}
	_ = out.Finish()
	if strings.Contains(buf.String(), "should-not-appear") {
		t.Fatalf("discard failed: %q", buf.String())
	}
}

func TestItem_CaptureBindsEvidence(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "gate", Stdout: &buf, Stderr: &buf})
	docker := out.Task("docker daemon")
	cap := docker.Capture()
	_, _ = cap.Stderr().Write([]byte("Cannot connect to the Docker daemon"))
	docker.Fail("could not inspect the daemon", cap.DetailTail())
	_ = out.Finish()
	if !strings.Contains(buf.String(), "Cannot connect") {
		t.Fatalf("item capture detail missing:\n%s", buf.String())
	}
}
