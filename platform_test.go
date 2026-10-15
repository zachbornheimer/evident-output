package evo

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
)

type platformSecretRedactor struct{}

func (platformSecretRedactor) RedactString(s string) string {
	return strings.ReplaceAll(s, "SECRET_TOKEN", "[REDACTED]")
}

func TestCapture_RedactsOnRetention(t *testing.T) {
	var primary bytes.Buffer
	out := initRun(Config{Name: "sec", Facades: Facades{Stdout: &primary, Stderr: &primary, Redactor: platformSecretRedactor{}}})
	task := out.Task("fetch")
	cap := task.capture()
	_, _ = fmt.Fprintln(cap, "Authorization: Bearer SECRET_TOKEN")
	_ = cap.Close()

	if strings.Contains(cap.Text(), "SECRET_TOKEN") {
		t.Fatalf("token must be redacted in retention: %q", cap.Text())
	}
	if !strings.Contains(cap.Text(), "[REDACTED]") {
		t.Fatalf("expected redaction marker: %q", cap.Text())
	}

	task.fail("denied", detailTail(cap))
	_ = out.finish()
	if strings.Contains(primary.String(), "SECRET_TOKEN") {
		t.Fatalf("token leaked into presentation:\n%s", primary.String())
	}
}

func TestEntityID_DerivedKeyInSnapshotAndJSON(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Name: "id", Facades: Facades{Stdout: &buf, Stderr: &buf}})
	succeed(out.Task("working tree"))
	succeed(out.Task("download base"))
	_ = out.finish()

	snap := out.snapshot()
	if len(snap.Tasks) != 2 || snap.Tasks[0].Key != "task:/working tree" {
		t.Fatalf("first task key: %+v", snap.Tasks)
	}
	if snap.Tasks[1].Key != "task:/download base" {
		t.Fatalf("second task key: %+v", snap.Tasks)
	}

	tasks := out.decodedRunDocument(t).Data.Tasks
	if len(tasks) != 2 || tasks[0].Key != "task:/working tree" {
		t.Fatalf("json first task key: %+v", tasks)
	}
	if tasks[1].Key != "task:/download base" {
		t.Fatalf("json second task key: %+v", tasks)
	}
}

func TestResultWriter_FormatDataPurity(t *testing.T) {
	var human, result bytes.Buffer
	out := initRun(Config{Name: "build", Facades: Facades{Stdout: &result, Stderr: &human}})
	succeed(out.Task("compile"))
	succeed(out.Task("link"), "bin/app")
	if _, err := io.WriteString(out.ResultWriter(), `{"artifact":"bin/app"}`+"\n"); err != nil {
		t.Fatal(err)
	}
	_ = out.finish()

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

func TestResultWriter_WritesStdoutOnly(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := initRun(Config{Name: "h", Facades: Facades{Stdout: &stdout, Stderr: &stderr}})
	n, err := out.ResultWriter().Write([]byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	if n != len("payload") {
		t.Fatalf("write n=%d", n)
	}
	_ = out.finish()
	if stdout.String() != "payload" {
		t.Fatalf("stdout = %q, want the payload alone", stdout.String())
	}
	if strings.Contains(stderr.String(), "payload") {
		t.Fatalf("the payload reached stderr: %q", stderr.String())
	}
}

func TestItem_CaptureBindsEvidence(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Name: "gate", Facades: Facades{Stdout: &buf, Stderr: &buf}})
	docker := out.Task("docker daemon")
	cap := docker.capture()
	_, _ = cap.Stderr().Write([]byte("Cannot connect to the Docker daemon"))
	docker.fail("could not inspect the daemon", detailTail(cap))
	_ = out.finish()
	if !strings.Contains(buf.String(), "Cannot connect") {
		t.Fatalf("item capture detail missing:\n%s", buf.String())
	}
}
