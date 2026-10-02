package main

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func entry(id, section string, tier Tier, tests ...TestRef) Entry {
	return Entry{ID: id, Section: section, Quote: "q", Tier: tier, Tests: tests}
}

func ref(pkg, run string) TestRef { return TestRef{Pkg: pkg, Run: run} }

func TestScoreStatuses(t *testing.T) {
	runner := &fakeRunner{
		streams: map[string]string{
			"./a": event("pass", "TestPass") + event("fail", "TestFail") + event("skip", "TestSkip") + event("pass", "TestPass2"),
		},
		errs: map[string]error{"./broken": errors.New("build failed")},
	}
	waived := entry("C01-006", "1", Tier12)
	waived.Waiver = "manual"
	entries := []Entry{
		entry("C01-001", "1", Tier11, ref("./a", "TestPass")),
		entry("C01-002", "1", Tier11, ref("./a", "TestFail")),
		entry("C01-003", "1", Tier11, ref("./a", "TestSkip")),
		entry("C01-004", "1", Tier11, ref("./a", "TestMissing")),
		entry("C01-005", "2", Tier12, ref("./broken", "TestX")),
		waived,
		entry("C01-007", "2", Tier12, ref("./a", "TestPass"), ref("./a", "TestFail")),
		entry("C01-008", "2", Tier12, ref("./a", "TestPass"), ref("./a", "TestPass2")),
	}
	score := scoreEntries(entries, runner, "r", "evopending")
	want := map[string]Status{
		"C01-001": StatusPass, "C01-002": StatusFail, "C01-003": StatusFail, "C01-004": StatusFail,
		"C01-005": StatusFail, "C01-006": StatusWaived, "C01-007": StatusFail, "C01-008": StatusPass,
	}
	if !reflect.DeepEqual(score.IDs, want) {
		t.Fatalf("statuses\n got %v\nwant %v", score.IDs, want)
	}
}

func TestScoreRunsEachPackageOnce(t *testing.T) {
	runner := &fakeRunner{streams: map[string]string{"./a": event("pass", "TestA") + event("pass", "TestB")}}
	entries := []Entry{
		entry("C01-001", "1", Tier11, ref("./a", "TestA")),
		entry("C01-002", "1", Tier11, ref("./a", "TestB"), ref("./a", "TestA")),
	}
	scoreEntries(entries, runner, "r", "evopending")
	if want := []string{"./a TestA,TestB"}; !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls %v, want %v", runner.calls, want)
	}
}

func TestScoreTierAndSectionTotals(t *testing.T) {
	runner := &fakeRunner{streams: map[string]string{"./a": event("pass", "TestA")}}
	waived := entry("C02-003", "2", Tier12)
	waived.Waiver = "manual"
	entries := []Entry{
		entry("C01-001", "1", Tier11, ref("./a", "TestA")),
		entry("C01-002", "1", Tier11, ref("./a", "TestNope")),
		entry("C02-001", "2", Tier12, ref("./a", "TestA")),
		waived,
	}
	score := scoreEntries(entries, runner, "r", "evopending")
	if got, want := score.Tiers[Tier11], (TierTotals{Pass: 1, Fail: 1, Total: 2}); got != want {
		t.Fatalf("tier 1.1 %+v, want %+v", got, want)
	}
	if got, want := score.Tiers[Tier12], (TierTotals{Pass: 1, Waived: 1, Total: 2}); got != want {
		t.Fatalf("tier 1.2 %+v, want %+v", got, want)
	}
	if got, want := score.Sections["1"], (SectionTotals{Pass: 1, Total: 2}); got != want {
		t.Fatalf("section 1 %+v, want %+v", got, want)
	}
	lines := strings.Join(summaryLines(score), "\n")
	if !strings.Contains(lines, "tier 1.1: 1 pass, 1 fail, 0 waived of 2") {
		t.Fatalf("summary %q", lines)
	}
}

func TestParseOutcomesIgnoresNoiseAndSubtests(t *testing.T) {
	stream := "go: downloading x\n" + event("pass", "TestA") + `{"Action":"output","Test":"TestA"}` + "\n" + event("fail", "TestB")
	got := parseOutcomes([]byte(stream))
	want := map[string]string{"TestA": "pass", "TestB": "fail"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func specFixture(ratchet string, stream string) (*fakeFS, *fakeRunner) {
	files := map[string]string{
		"r/" + contractPath: "the quote text",
		"r/conformance/spec/req/s01.json": `[
 {"id":"C01-001","section":"1","quote":"the quote","tier":"1.1","tests":[{"pkg":"./a","run":"TestA"}]},
 {"id":"C01-002","section":"1","quote":"quote text","tier":"1.1","tests":[{"pkg":"./a","run":"TestB"}]}]`,
	}
	if ratchet != "" {
		files["r/"+ratchetPath] = ratchet
	}
	return newFakeFS(files), &fakeRunner{streams: map[string]string{"./a": stream}}
}

func TestGateFailsOnRegression(t *testing.T) {
	fsys, runner := specFixture(`["C01-001","C01-002"]`, event("pass", "TestA")+event("fail", "TestB"))
	var out bytes.Buffer
	code, err := execute(options{lint: true, gate: true, root: "r", scoreOut: defaultScoreOut, tags: defaultTags}, fsys, runner, &out)
	if err != nil || code != 1 {
		t.Fatalf("code %d err %v, want 1 nil", code, err)
	}
	if !strings.Contains(out.String(), "C01-002: ratcheted requirement is not passing") || strings.Contains(out.String(), "C01-001:") {
		t.Fatalf("output %q", out.String())
	}
	if _, ok := fsys.files["r/"+defaultScoreOut]; !ok {
		t.Fatal("score.json not written")
	}
}

func TestGatePassesWhenRatchetHolds(t *testing.T) {
	fsys, runner := specFixture(`["C01-001"]`, event("pass", "TestA")+event("fail", "TestB"))
	code, err := execute(options{gate: true, root: "r", scoreOut: defaultScoreOut, tags: defaultTags}, fsys, runner, &bytes.Buffer{})
	if err != nil || code != 0 {
		t.Fatalf("code %d err %v, want 0 nil", code, err)
	}
}

func TestLintFailureExitsOneWithoutScoring(t *testing.T) {
	fsys, runner := specFixture("", event("pass", "TestA"))
	fsys.files["r/"+contractPath] = "other"
	code, err := execute(options{lint: true, gate: true, root: "r", scoreOut: defaultScoreOut, tags: defaultTags}, fsys, runner, &bytes.Buffer{})
	if err != nil || code != 1 || len(runner.calls) != 0 {
		t.Fatalf("code %d err %v calls %v", code, err, runner.calls)
	}
}

func TestUpdateOnlyAddsNeverRemoves(t *testing.T) {
	fsys, runner := specFixture(`["C01-002","C09-001"]`, event("pass", "TestA")+event("fail", "TestB"))
	_, err := execute(options{update: true, root: "r", scoreOut: defaultScoreOut, tags: defaultTags}, fsys, runner, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := loadRatchet(fsys, "r")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"C01-001", "C01-002", "C09-001"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ratchet %v, want %v", got, want)
	}
}

func TestMissingRatchetIsEmpty(t *testing.T) {
	got, err := loadRatchet(newFakeFS(map[string]string{}), "r")
	if err != nil || got != nil {
		t.Fatalf("got %v %v", got, err)
	}
}
