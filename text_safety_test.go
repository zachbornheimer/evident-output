package evo_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

func TestTXT008_OSCNeutralized(t *testing.T) {
	// OSC 8 introducer ESC ]
	s := txt.Text("x\x1b]8;;http://evil\x07y")
	if strings.Contains(s, "\x1b") {
		t.Fatal(s)
	}
}

func TestTXT009_CRLFNeutralized(t *testing.T) {
	s := txt.Text("a\rb\bc")
	if strings.ContainsAny(s, "\r\b") {
		t.Fatal(s)
	}
}

func TestTXT010_NewlineInNameNormalized(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	it := out.Task("a\nb")
	if strings.Contains(it.Snapshot().Name, "\n") {
		t.Fatal(it.Snapshot().Name)
	}
}

func TestTXT020_EmptyNameStillCreates(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task(""))
	_ = out.Finish()
}

func TestTXT001_ASCIIWidthStable(t *testing.T) {
	var wide, narrow bytes.Buffer
	mk := func(w io.Writer, width int) {
		out := evo.Init(evo.Config{Isolated: true, Stdout: w, Title: "s", Width: width, Color: evo.ColorNever, Plain: true})
		commit(out.Task("c"),
			evo.EffectSpec{Verb: evo.EffectAdd, Object: "x", Quantity: 1},
			evo.EffectSpec{Verb: evo.EffectCreate, Object: "f", Quantity: 1},
		)
		_ = out.Finish()
		_ = out.Close()
	}
	mk(&wide, 80)
	mk(&narrow, 30)
	if wide.String() == narrow.String() {
		t.Fatal("expected width to change layout")
	}
	if !strings.Contains(narrow.String(), "added 1 x") {
		t.Fatal(narrow.String())
	}
}

func TestTXT012_LongPathTruncationPolicy(t *testing.T) {
	long := strings.Repeat("a", 200) + "/file.go"
	got := txt.Truncate(long, 40)
	if txt.Cells(got) > 40 {
		t.Fatal(got, txt.Cells(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatal(got)
	}
}

func TestTXT017_DuplicateNamesReadable(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	// Output.Task get-or-creates by name (L1); two distinct rows sharing a
	// display name need distinct evo.ID.
	succeed(out.Task("same"))
	out.Task("same").Block("x")
	_ = out.Finish()
	if strings.Count(buf.String(), "same") < 2 {
		t.Fatal(buf.String())
	}
}

func TestTXT018_BidiInNames(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	it := out.Task("ok\u202Ebad")
	if strings.ContainsRune(it.Snapshot().Name, '\u202e') {
		t.Fatal(it.Snapshot().Name)
	}
}

func TestTXT013_ANSIWidthParity(t *testing.T) {
	plain := "hello world"
	styled := "\x1b[31mhello world\x1b[0m"
	if txt.VisibleCells(plain) != txt.VisibleCells(styled) {
		t.Fatalf("plain=%d styled=%d", txt.VisibleCells(plain), txt.VisibleCells(styled))
	}
}

func TestTXT014_OSC8ZeroCells(t *testing.T) {
	link := "\x1b]8;;https://example.com\x07click\x1b]8;;\x07"
	if txt.VisibleCells(link) != txt.Cells("click") {
		t.Fatalf("got %d want %d", txt.VisibleCells(link), txt.Cells("click"))
	}
}

func TestTXT015_NarrowStackDetailParent(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Title: "repo", Width: 28, Color: evo.ColorNever, Plain: true})
	out.Task("working tree").Block("dirty", evo.Detail("commit or stash"))
	succeed(out.Task("remote"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	// Detail must appear after working tree and before remote's terminal ok line order.
	iTree := strings.Index(s, "working tree")
	iDetail := strings.Index(s, "commit or stash")
	iRemote := strings.Index(s, "remote")
	if iTree < 0 || iDetail < 0 || iRemote < 0 || (iTree >= iDetail || iDetail >= iRemote) {
		t.Fatalf("detail not associated with parent:\n%s", s)
	}
}

func TestTXT016_LeaderBoundedAndOmittedNarrow(t *testing.T) {
	var wide, narrow bytes.Buffer
	mk := func(w io.Writer, cols int) {
		out := evo.Init(evo.Config{Isolated: true, Stdout: w, Title: "x", Width: cols, Color: evo.ColorNever, Plain: true})
		ch := out.Task("files")
		ch.Define(effectOf(evo.EffectAdd, "a.go", 1))
		ch.Define(effectOf(evo.EffectRemove, "b.go", 2))
		_ = out.Finish()
		_ = out.Close()
	}
	mk(&wide, 80)
	mk(&narrow, 30)
	if strings.Contains(narrow.String(), "·") {
		t.Fatalf("narrow should omit leaders: %q", narrow.String())
	}
	// Wide may use leaders when verb lengths differ; either form is OK if bounded.
	if n := strings.Count(wide.String(), "·"); n > 24 {
		t.Fatalf("unbounded leaders: %d in %q", n, wide.String())
	}
}

// TestTXT019_ManyProblemsBounded's premise (attach 200 structured Problems
// via one bulk verb call) no longer has a public construction path — a Task
// verb now produces exactly one Problem per resolution. The storage-side
// invariant it pinned (Snapshot retains every Problem, not just the plain
// projection's display bound) is covered directly against a hand-built
// Snapshot by TestHumanProblemList_IsBounded (problem_bound_test.go).
