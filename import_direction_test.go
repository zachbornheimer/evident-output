package evo_test

import (
	"go/parser"
	"go/token"
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
		"WithTimeout":  true,
		"WithDeadline": true,
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
	want := []string{"AppendLine", "SetSummary", "Transition", "RecordRunFact", "MarkSectionStreamed", "ClearAll"}
	var got []string
	for _, hit := range found {
		got = append(got, hit[strings.LastIndex(hit, " ")+1:])
	}
	if !slices.Equal(got, want) {
		t.Errorf("flagged %v, want %v", got, want)
	}
}
