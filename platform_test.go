package evo_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

type secretRedactor struct{}

func (secretRedactor) RedactString(s string) string {
	return strings.ReplaceAll(s, "SECRET_TOKEN", "[REDACTED]")
}

func TestCapture_RedactsOnRetention(t *testing.T) {
	var primary bytes.Buffer
	out := evo.New(evo.Config{
		Title:    "sec",
		Stdout:   &primary,
		Stderr:   &primary,
		Redactor: secretRedactor{},
	})
	task := out.Task("fetch")
	cap := task.Capture()
	fmt.Fprintln(cap, "Authorization: Bearer SECRET_TOKEN")
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
	out := evo.New(evo.Config{Title: "id", Stdout: &buf, Stderr: &buf})
	out.Item("working tree", evo.ID("gate.working-tree")).OK()
	out.Task("download base", evo.ID("build.base.download")).Done()
	_ = out.Finish()

	snap := out.Snapshot()
	if len(snap.Items) != 1 || snap.Items[0].Key != "gate.working-tree" {
		t.Fatalf("item key: %+v", snap.Items)
	}
	if len(snap.Tasks) != 1 || snap.Tasks[0].Key != "build.base.download" {
		t.Fatalf("task key: %+v", snap.Tasks)
	}

	raw, err := evo.EncodeJSON(snap)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	items, _ := doc["items"].([]any)
	item0, _ := items[0].(map[string]any)
	if item0["key"] != "gate.working-tree" {
		t.Fatalf("json item key: %#v", item0)
	}
	tasks, _ := doc["tasks"].([]any)
	task0, _ := tasks[0].(map[string]any)
	if task0["key"] != "build.base.download" {
		t.Fatalf("json task key: %#v", task0)
	}
}

func TestEntityID_DuplicateIsMisuse(t *testing.T) {
	var buf bytes.Buffer
	out := evo.New(evo.Config{Title: "dup", Stdout: &buf, Stderr: &buf})
	out.Item("a", evo.ID("same")).OK()
	out.Item("b", evo.ID("same")).OK()
	if out.Err() == nil {
		t.Fatal("expected ErrDuplicateKey misuse")
	}
}

func TestScope_QualifiesKeys(t *testing.T) {
	var buf bytes.Buffer
	out := evo.New(evo.Config{Title: "scope", Stdout: &buf, Stderr: &buf})
	reg := out.Scope("registry")
	reg.Item("credentials", evo.ID("auth")).OK()
	reg.Task("pull", evo.ID("image.pull")).Done()
	// Already-qualified IDs are not double-prefixed.
	reg.Item("ready", evo.ID("registry.ready")).OK()
	_ = out.Finish()

	snap := out.Snapshot()
	keys := map[string]bool{}
	for _, it := range snap.Items {
		keys[it.Key] = true
	}
	for _, tk := range snap.Tasks {
		keys[tk.Key] = true
	}
	if !keys["registry.auth"] || !keys["registry.image.pull"] || !keys["registry.ready"] {
		t.Fatalf("scope keys: %v", keys)
	}
}

func TestResultWriter_FormatDataPurity(t *testing.T) {
	var human, result bytes.Buffer
	out := evo.New(evo.Config{
		Title:  "build",
		Format: evo.FormatData,
		Stdout: &result,
		Stderr: &human,
	})
	out.Item("compile").OK()
	out.Task("link").Done("bin/app")
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
	out := evo.New(evo.Config{Title: "h", Stdout: &buf, Stderr: &buf})
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

func TestScope_WriterAndSlogHandlerNonNil(t *testing.T) {
	var buf bytes.Buffer
	out := evo.New(evo.Config{Title: "s", Stdout: &buf, Stderr: &buf})
	sc := out.Scope("plugin")
	if sc.Writer() == nil {
		t.Fatal("Writer nil")
	}
	if sc.SlogHandler(nil) == nil {
		t.Fatal("SlogHandler nil")
	}
	if sc.Name() != "plugin" {
		t.Fatalf("name %q", sc.Name())
	}
}
