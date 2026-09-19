package evo_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/apisurface"
)

func TestAPIContract_RequiredNameAbsentFromFakeSurfaceFails(t *testing.T) {
	t.Parallel()
	live := []string{"func Init()", "type Output"}
	golden := []string{"func Init()", "type Output"}
	required := []string{"File", "Init"}
	got := apisurface.Check(live, golden, required, nil)
	if got.OK() {
		t.Fatal("want failure when required File is absent even if golden matches")
	}
	if !slices.Contains(got.RequiredMissing, "File") {
		t.Fatalf("required-missing want File, got %v", got.RequiredMissing)
	}
	if slices.Contains(got.RequiredMissing, "Init") {
		t.Fatalf("Init is on the fake surface; required-missing=%v", got.RequiredMissing)
	}
}

func TestAPIContract_ExtraLineVsGoldenFails(t *testing.T) {
	t.Parallel()
	live := []string{"func Extra()", "func Init()"}
	golden := []string{"func Init()"}
	got := apisurface.Check(live, golden, nil, nil)
	if got.OK() {
		t.Fatal("want failure when live has a line golden does not")
	}
	if !slices.Contains(got.Extra, "func Extra()") {
		t.Fatalf("extra want func Extra(), got %v", got.Extra)
	}
}

func TestAPIContract_RetiredPresentEvenIfGoldenRewrittenFails(t *testing.T) {
	t.Parallel()
	live := []string{"func Init()", "func MainWith()"}
	golden := []string{"func Init()", "func MainWith()"}
	got := apisurface.Check(live, golden, nil, apisurface.RetiredNames)
	if got.OK() {
		t.Fatal("want failure when a retired name is present even if golden matches")
	}
	if !slices.Contains(got.RetiredPresent, "MainWith") {
		t.Fatalf("retired-present want MainWith, got %v", got.RetiredPresent)
	}
}

func TestAPIContract_FileRequirementDoesNotPassOnFileSpecAlone(t *testing.T) {
	t.Parallel()
	live := []string{"func Init()", "type FileSpec"}
	golden := []string{"func Init()", "type FileSpec"}
	got := apisurface.Check(live, golden, []string{"File"}, nil)
	if got.OK() {
		t.Fatal("type FileSpec must not satisfy required token File")
	}
	if !slices.Contains(got.RequiredMissing, "File") {
		t.Fatalf("required-missing want File, got %v", got.RequiredMissing)
	}
}

func TestAPIContract_RequiredFileListsSpecFloor(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(apisurface.RequiredRelPath)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]struct{}{}
	for line := range strings.SplitSeq(strings.TrimRight(string(raw), "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		got[line] = struct{}{}
	}
	floor := []string{
		"Init", "Run", "Main", "Task", "Group", "Sequence", "Define",
		"File", "Exec", "FSPath", "Fact", "Record", "Verify",
		"func (TaskHandle) After", "FileSpec", "ExecSpec",
		"Patch", "PatchSpec", "PatchResult", "Fingerprint", "Value", "App",
	}
	for _, name := range floor {
		if _, ok := got[name]; !ok {
			t.Errorf("testdata/api_required.txt missing spec floor token %q", name)
		}
	}
}
