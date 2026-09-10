package modpin

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestRequireVersion_LineAndBlock(t *testing.T) {
	t.Parallel()
	line := "module app\n\nrequire github.com/zachbornheimer/evident-output v0.4.6\n"
	if got := RequireVersion(line, ModulePath); got != "v0.4.6" {
		t.Fatalf("line form: got %q", got)
	}
	block := "module app\nrequire (\n\tgithub.com/zachbornheimer/evident-output v0.4.2 // indirect\n)\n"
	if got := RequireVersion(block, ModulePath); got != "v0.4.2" {
		t.Fatalf("block form: got %q", got)
	}
	if got := RequireVersion("module app\n", ModulePath); got != "" {
		t.Fatalf("missing require: got %q", got)
	}
}

func TestParse_SelfModule(t *testing.T) {
	t.Parallel()
	src := "module github.com/zachbornheimer/evident-output\n\ngo 1.25.0\n"
	pin, err := Parse(src, "/src/evident-output")
	if err != nil {
		t.Fatal(err)
	}
	if !pin.SelfModule {
		t.Fatal("expected SelfModule")
	}
	if pin.Version != "" || pin.ReplacePath != "" {
		t.Fatalf("self module should have no require/replace, got %+v", pin)
	}
}

func TestParse_RelativeReplaceResolved(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "app")
	src := "module app\n\nrequire github.com/zachbornheimer/evident-output v0.4.6\nreplace github.com/zachbornheimer/evident-output => ../evo\n"
	pin, err := Parse(src, dir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Clean(filepath.Join(dir, "../evo"))
	if pin.ReplacePath != want {
		t.Fatalf("ReplacePath=%q want %q", pin.ReplacePath, want)
	}
	if pin.Version != "v0.4.6" {
		t.Fatalf("Version=%q", pin.Version)
	}
}

func TestParse_NonPathReplaceRejected(t *testing.T) {
	t.Parallel()
	src := "module app\nreplace github.com/zachbornheimer/evident-output => github.com/other/evident-output v1.2.3\n"
	_, err := Parse(src, "/tmp/app")
	if !errors.Is(err, ErrNonPathReplace) {
		t.Fatalf("got %v, want ErrNonPathReplace", err)
	}
}
