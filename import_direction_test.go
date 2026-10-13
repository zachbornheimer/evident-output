package evo_test

import (
	"errors"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
)

// systemImports are import paths no non-facade internal package may use.
// An entry ending in "/" forbids the whole subtree.
var systemImports = []string{
	"os",
	"os/exec",
	"os/signal",
	"os/user",
	"io/ioutil",
	"syscall",
	"golang.org/x/term",
	"golang.org/x/sys/",
}

// wallClockCalls are the time-package functions that read or wait on the wall
// clock. Duration and Time arithmetic stay allowed.
var wallClockCalls = map[string]bool{
	"Now":       true,
	"Since":     true,
	"Until":     true,
	"Sleep":     true,
	"After":     true,
	"AfterFunc": true,
	"NewTimer":  true,
	"NewTicker": true,
	"Tick":      true,
}

// bannedCalls maps an import path to the functions of it that touch the
// system outside the facades: the wall clock, the working directory and
// symlinks behind path resolution, directory walks and globs, and context
// deadlines (which read the wall clock).
var bannedCalls = map[string]map[string]bool{
	"time": wallClockCalls,
	"path/filepath": {
		"Abs":          true,
		"EvalSymlinks": true,
		"Glob":         true,
		"Walk":         true,
		"WalkDir":      true,
	},
	"context": {
		"WithTimeout":       true,
		"WithTimeoutCause":  true,
		"WithDeadline":      true,
		"WithDeadlineCause": true,
	},
}

func isSystemImport(importPath string) bool {
	for _, banned := range systemImports {
		if importPath == banned || (strings.HasSuffix(banned, "/") && strings.HasPrefix(importPath, banned)) {
			return true
		}
	}
	return false
}

// TestOnlyFacadesTouchTheSystem fails when an internal package outside the
// four facades imports a system package (os, os/exec, os/signal, os/user,
// io/ioutil, syscall, x/term, x/sys) or references a banned function
// (wall-clock, path resolution, walk, glob, context deadline), called or
// taken as a value.
func TestOnlyFacadesTouchTheSystem(t *testing.T) {
	violations, _, err := scanProductionFiles("internal", isFacadePackage, func(file string) ([]string, error) {
		return touches(file, isSystemImport, bannedCalls)
	})
	if err != nil {
		t.Fatalf("scan internal/: %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("%d places outside the facades touch the system:\n  %s",
			len(violations), strings.Join(violations, "\n  "))
	}
}

// TestFacadesImportOnlyTheClock fails when internal/fs, internal/process or
// internal/terminal imports another package of this module other than
// internal/clock. The facades sit at the bottom of the layout; an import of
// anything above would invert it.
func TestFacadesImportOnlyTheClock(t *testing.T) {
	var violations []string
	for _, facade := range facadePackages {
		if facade == clockPackage {
			continue
		}
		found, checked, err := scanProductionFiles(facade, skipNothing, func(file string) ([]string, error) {
			return touches(file, isNonClockModuleImport, nil)
		})
		if err != nil {
			t.Fatalf("scan %s: %v", facade, err)
		}
		if checked == 0 {
			t.Fatalf("%s holds no Go files", facade)
		}
		violations = append(violations, found...)
	}
	if len(violations) > 0 {
		t.Errorf("%d facade imports reach above the clock:\n  %s",
			len(violations), strings.Join(violations, "\n  "))
	}
}

// TestOnlyTheClockReadsTheWallClock fails when any internal package other
// than internal/clock, facades included, references a function of package
// time that reads or waits on the wall clock.
func TestOnlyTheClockReadsTheWallClock(t *testing.T) {
	timeOnly := map[string]map[string]bool{"time": wallClockCalls}
	violations, _, err := scanProductionFiles("internal",
		func(dir string) bool { return dir == clockPackage },
		func(file string) ([]string, error) { return touches(file, neverBanned, timeOnly) })
	if err != nil {
		t.Fatalf("scan internal/: %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("%d places outside %s read the wall clock:\n  %s",
			len(violations), clockPackage, strings.Join(violations, "\n  "))
	}
}

// recordPackage is the one package that owns run truth.
const recordPackage = "internal/record"

// TestRecordImportsOnlyFacades fails when internal/record is missing, or when
// a non-test file under it imports anything but the standard library and the
// four facades. record sits beneath graph, freshness, change and project, so
// an import of any of them (or of the packages they grew from) would invert
// the direction the layout promises.
func TestRecordImportsOnlyFacades(t *testing.T) {
	violations, checked, err := scanProductionFiles(recordPackage, skipNothing, func(file string) ([]string, error) {
		return touches(file, isNonFacadeImport, nil)
	})
	if err != nil {
		t.Fatalf("scan %s: %v", recordPackage, err)
	}
	if checked == 0 {
		t.Fatalf("%s holds no Go files", recordPackage)
	}
	if len(violations) > 0 {
		t.Errorf("%d imports under %s are neither standard library nor a facade:\n  %s",
			len(violations), recordPackage, strings.Join(violations, "\n  "))
	}
}

// graphPackage decides when work runs.
const graphPackage = "internal/graph"

// TestGraphImportsOnlyRecordAndFacades fails when internal/graph is missing,
// or when a non-test file under it imports a package of this module other
// than record, misuse and the facades. graph sits beside freshness and
// change, above record: reaching into engine, render, core or a sibling
// would let scheduling depend on presentation or on what it schedules.
func TestGraphImportsOnlyRecordAndFacades(t *testing.T) {
	violations, checked, err := scanProductionFiles(graphPackage, skipNothing, func(file string) ([]string, error) {
		return touches(file, isGraphForbiddenImport, nil)
	})
	if err != nil {
		t.Fatalf("scan %s: %v", graphPackage, err)
	}
	if checked == 0 {
		t.Fatalf("%s holds no Go files", graphPackage)
	}
	if len(violations) > 0 {
		t.Errorf("%d imports under %s are neither record, misuse nor a facade:\n  %s",
			len(violations), graphPackage, strings.Join(violations, "\n  "))
	}
}

// freshnessPackage decides whether work is needed.
const freshnessPackage = "internal/freshness"

// TestFreshnessImportsOnlyRecordGraphAndFacades fails when internal/freshness
// is missing, or when a non-test file under it imports a package of this
// module other than record, graph and the facades. freshness sits beside
// graph and change, above record: reaching into engine, change, render, core,
// wire or resource would let the decision whether to act depend on
// presentation or on how the act is performed.
func TestFreshnessImportsOnlyRecordGraphAndFacades(t *testing.T) {
	violations, checked, err := scanProductionFiles(freshnessPackage, skipNothing, func(file string) ([]string, error) {
		return touches(file, isFreshnessForbiddenImport, nil)
	})
	if err != nil {
		t.Fatalf("scan %s: %v", freshnessPackage, err)
	}
	if checked == 0 {
		t.Fatalf("%s holds no Go files", freshnessPackage)
	}
	if len(violations) > 0 {
		t.Errorf("%d imports under %s are neither record, graph nor a facade:\n  %s",
			len(violations), freshnessPackage, strings.Join(violations, "\n  "))
	}
}

// projectPackage decides how a run is shown: live TTY, plain text, and wire.
const projectPackage = "internal/project"

// misusePackage owns misuse codes, remedy text and the first-misuse store.
const misusePackage = "internal/misuse"

// TestMisuseImportsOnlyRecordAndFacades fails when internal/misuse is
// missing, or when a non-test file under it imports a package of this
// module other than record and the facades. misuse sits beside graph,
// freshness, change and project, above record: reaching into any of them
// would let recording depend on scheduling or presentation.
func TestMisuseImportsOnlyRecordAndFacades(t *testing.T) {
	violations, checked, err := scanProductionFiles(misusePackage, skipNothing, func(file string) ([]string, error) {
		return touches(file, isMisuseForbiddenImport, nil)
	})
	if err != nil {
		t.Fatalf("scan %s: %v", misusePackage, err)
	}
	if checked == 0 {
		t.Fatalf("%s holds no Go files", misusePackage)
	}
	if len(violations) > 0 {
		t.Errorf("%d imports under %s are neither record nor a facade:\n  %s",
			len(violations), misusePackage, strings.Join(violations, "\n  "))
	}
}

// TestProjectImportsOnlyRecordAndFacades fails when internal/project is
// missing, or when a non-test file under it imports a package of this
// module other than record, the facades, and its own subpackages. project
// sits above record and beside graph, freshness and change: reaching into
// engine, graph, freshness, change, core, text, render or wire would let
// presentation depend on scheduling or on how a change is performed.
func TestProjectImportsOnlyRecordAndFacades(t *testing.T) {
	violations, checked, err := scanProductionFiles(projectPackage, skipNothing, func(file string) ([]string, error) {
		return touches(file, isProjectForbiddenImport, nil)
	})
	if err != nil {
		t.Fatalf("scan %s: %v", projectPackage, err)
	}
	if checked == 0 {
		t.Fatalf("%s holds no Go files", projectPackage)
	}
	if len(violations) > 0 {
		t.Errorf("%d imports under %s are neither record, a facade, nor a project subpackage:\n  %s",
			len(violations), projectPackage, strings.Join(violations, "\n  "))
	}
}

// changePackage decides what is done.
const changePackage = "internal/change"

// TestChangeImportsOnlyRecordGraphFreshnessAndFacades fails when
// internal/change is missing, or when a non-test file under it imports a
// package of this module other than record, graph, freshness and the
// facades. change sits beside graph and freshness, above record: reaching
// into engine, render, core, wire or the packages it grew from (patch,
// publish, resource) would let performing a change depend on presentation
// or on how it is scheduled. What it needs of the run around it leaves
// through change.Host, a consumer-defined interface the engine implements.
func TestChangeImportsOnlyRecordGraphFreshnessAndFacades(t *testing.T) {
	violations, checked, err := scanProductionFiles(changePackage, skipNothing, func(file string) ([]string, error) {
		return touches(file, isChangeForbiddenImport, nil)
	})
	if err != nil {
		t.Fatalf("scan %s: %v", changePackage, err)
	}
	if checked == 0 {
		t.Fatalf("%s holds no Go files", changePackage)
	}
	if len(violations) > 0 {
		t.Errorf("%d imports under %s are neither record, graph, freshness nor a facade:\n  %s",
			len(violations), changePackage, strings.Join(violations, "\n  "))
	}
}

// graphImporters are the only packages that may import internal/graph: the
// engine that wires it today, and the packages that report into it later.
var graphImporters = []string{
	"internal/engine",
	"internal/change",
	"internal/freshness",
}

// TestOnlyEngineChangeAndFreshnessImportGraph fails when any other package
// under internal/ imports internal/graph. record, project, misuse and the
// facades sit beside or beneath it and must not depend on how work is
// scheduled.
func TestOnlyEngineChangeAndFreshnessImportGraph(t *testing.T) {
	violations, _, err := scanProductionFiles("internal",
		func(dir string) bool { return dir == graphPackage || slices.Contains(graphImporters, dir) },
		func(file string) ([]string, error) { return touches(file, isGraphImport, nil) })
	if err != nil {
		t.Fatalf("scan internal/: %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("%d imports of %s come from outside engine, change and freshness:\n  %s",
			len(violations), graphPackage, strings.Join(violations, "\n  "))
	}
}

// recordProducerPackages are the only packages, besides record itself, that
// may call a method that writes run truth into record.
var recordProducerPackages = []string{
	"internal/engine",
	"internal/graph",
	"internal/freshness",
	"internal/change",
}

func isRecordProducerOrOwner(dir string) bool {
	return dir == recordPackage || slices.Contains(recordProducerPackages, dir)
}

// TestOnlyProducersWriteRecord fails when a package other than record and the
// producers (engine, graph, freshness, change) calls a method that writes
// run truth: one whose name begins with Record, Append, Apply, Resolve, Set,
// Transition, Mark or Clear, on a record value. core, render, text, wire and
// the rest may read record, never write it. The scan has no type
// information; recordWriteCallsIn documents how it recognises a record value.
func TestOnlyMisuseRecordsMisuse(t *testing.T) {
	violations, _, err := scanProductionFiles("internal",
		func(dir string) bool { return dir == misusePackage },
		misuseRecording)
	if err != nil {
		t.Fatalf("scan internal/: %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("%d first-misuse stores live outside %s:\n  %s",
			len(violations), misusePackage, strings.Join(violations, "\n  "))
	}
}

func TestMisuseRecordingScanRecognisesTheShapes(t *testing.T) {
	const source = `package probe

type Output struct {
	misuseMu              int
	misuseSubject         string
	misuseRejectedSummary string
	misuse                error
	recorder              *Recorder
}

func (o *Output) keepMisuse() {}
func (o *Output) noteMisuse() {}
func (discardMisuse) RecordMisuseFor(string, error) {}
func (s sink) RecordMisuseFor(subject string, err error) { s.store(err) }
`
	file := path.Join(t.TempDir(), "probe.go")
	if err := os.WriteFile(file, []byte(source), 0o600); err != nil {
		t.Fatalf("write probe: %v", err)
	}
	found, err := misuseRecording(file)
	if err != nil {
		t.Fatalf("scan probe: %v", err)
	}
	var got []string
	for _, hit := range found {
		got = append(got, hit[strings.Index(hit, " ")+1:])
	}
	want := []string{
		"field misuseMu",
		"field misuseSubject",
		"field misuseRejectedSummary",
		"store misuse error",
		"declaration keepMisuse",
		"declaration noteMisuse",
		"declaration RecordMisuseFor",
	}
	if !slices.Equal(got, want) {
		t.Errorf("flagged %v, want %v", got, want)
	}
}

func TestOnlyProducersWriteRecord(t *testing.T) {
	violations, _, err := scanProductionFiles("internal", isRecordProducerOrOwner, recordWriteCalls)
	if err != nil {
		t.Fatalf("scan internal/: %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("%d record writes come from outside the producers:\n  %s",
			len(violations), strings.Join(violations, "\n  "))
	}
}

// TestRecordWriteScanRecognisesRecordValues pins the shapes the heuristic
// promises to flag, and the ones it promises to leave alone.
func TestRecordWriteScanRecognisesRecordValues(t *testing.T) {
	const source = `package probe

import "github.com/zachbornheimer/evident-output/internal/record"

type holder struct {
	journal *record.Run
	rec     *record.Task
	other   int
}

func writes(h *holder, task *record.Task, run record.Run) {
	h.journal.AppendLine("a")
	h.rec.SetSummary("b")
	task.Transition(record.Done)
	task.Settle(record.Failed)
	run.RecordRunFact(record.Fact{})
	built := record.NewRun()
	built.MarkSectionStreamed("x", 0)
	literal := &record.Run{}
	literal.ClearAll()
}

func reads(h *holder, task *record.Task, other fakeStore) {
	h.journal.Lines()
	task.Summary()
	other.AppendLine("not record")
	h.other.Set("not record")
}
`
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, "probe.go", source, 0)
	if err != nil {
		t.Fatalf("parse probe: %v", err)
	}
	found, err := recordWriteCallsIn(fset, parsed)
	if err != nil {
		t.Fatalf("scan probe: %v", err)
	}
	want := []string{"AppendLine", "SetSummary", "Transition", "Settle", "RecordRunFact", "MarkSectionStreamed", "ClearAll"}
	var got []string
	for _, hit := range found {
		got = append(got, hit[strings.LastIndex(hit, " ")+1:])
	}
	if !slices.Equal(got, want) {
		t.Errorf("flagged %v, want %v", got, want)
	}
}

// TestOnlyChangeHoldsClaims fails when any production file outside
// internal/change selects HoldResource or graph.ProcessHolds, or declares a
// HoldResource of its own. change owns the one claim registry and is the only
// code that reports a held claim to the scheduler's hold sensor, so a second
// holder would let two Runs of this process disagree about who owns a path.
// The tests of other packages may build fixtures around claims.
func TestOnlyChangeHoldsClaims(t *testing.T) {
	violations, _, err := scanProductionFiles(".", func(dir string) bool {
		return skipHiddenAndTestdata(dir) || dir == changePackage
	}, claimHolds)
	if err != nil {
		t.Fatalf("scan module: %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("%d claim holds outside %s:\n  %s",
			len(violations), changePackage, strings.Join(violations, "\n  "))
	}
}

// TestClaimHoldScanRecognisesTheShapes pins what the claim guard flags and
// what it leaves alone.
func TestClaimHoldScanRecognisesTheShapes(t *testing.T) {
	const source = `package probe

type registry struct{}

func (registry) HoldResource() {}

func holds(r registry) {
	r.HoldResource()
	defer graph.ProcessHolds().Enter().Leave()
	r.HoldRead()
}
`
	file := path.Join(t.TempDir(), "probe.go")
	if err := os.WriteFile(file, []byte(source), 0o600); err != nil {
		t.Fatalf("write probe: %v", err)
	}
	found, err := claimHolds(file)
	if err != nil {
		t.Fatalf("scan probe: %v", err)
	}
	var got []string
	for _, hit := range found {
		got = append(got, hit[strings.Index(hit, " ")+1:])
	}
	want := []string{"declaration HoldResource", "call HoldResource", "call ProcessHolds"}
	if !slices.Equal(got, want) {
		t.Errorf("flagged %v, want %v", got, want)
	}
}

// dissolvedPackages are the packages whose contents became part of
// internal/change. None may exist again, and nothing may import one.
var dissolvedPackages = []string{
	"internal/patch",
	"internal/publish",
	"internal/resource",
}

// TestDissolvedPackagesStayGone fails when a dissolved package's directory
// exists, or when any non-test Go file of the module imports one. The diff
// parser, the atomic publisher and the claim registry are internal/change's
// alone: a second copy beside it would be a second owner of the same logic.
func TestDissolvedPackagesStayGone(t *testing.T) {
	var problems []string
	for _, dissolved := range dissolvedPackages {
		if _, err := os.Stat(dissolved); !errors.Is(err, fs.ErrNotExist) {
			problems = append(problems, dissolved+" still exists")
		}
	}
	importers, _, err := scanProductionFiles(".", skipHiddenAndTestdata, func(file string) ([]string, error) {
		return touches(file, isDissolvedImport, nil)
	})
	if err != nil {
		t.Fatalf("scan module: %v", err)
	}
	problems = append(problems, importers...)
	if len(problems) > 0 {
		t.Errorf("%d references to packages folded into %s:\n  %s",
			len(problems), changePackage, strings.Join(problems, "\n  "))
	}
}

func isDissolvedImport(importPath string) bool {
	local, inModule := strings.CutPrefix(importPath, modulePath+"/")
	return inModule && slices.Contains(dissolvedPackages, local)
}

// skipHiddenAndTestdata skips version-control and tool directories and
// fixture trees, none of which hold the module's production code.
func skipHiddenAndTestdata(dir string) bool {
	base := path.Base(dir)
	return dir != "." && (strings.HasPrefix(base, ".") || base == "testdata")
}
