//go:build darwin || linux

package evo

import (
	"os"
	"strings"
	"testing"
)

const (
	asciiLocaleRuleSentence = "On a live terminal whose locale is not UTF-8, Human draws the bracketed forms; Plain always draws Unicode."
	asciiLocaleRuleKeyword  = "bracketed"
	asciiSucceededRow       = "[ok]"
	unicodeSucceededRow     = "✓"
)

// ptyRowsAfterOneTask runs one succeeding Task in the given Output on a pseudo-terminal in a C locale and
// returns the final screen.
func ptyRowsAfterOneTask(t *testing.T, output Output) string {
	t.Helper()
	t.Setenv("LC_ALL", "C")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "")
	pty := newPtyScreen(t)
	out := initRun(Config{Output: output, Facades: Facades{Stdout: pty.Stdout, Stderr: pty.Stderr}}, tuning{visibilityDelay: DelayForTest(0), color: colorNever})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("install"))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	_ = out.close()
	return strings.Join(pty.Lines(t), "\n")
}

// TestASCIILocaleGlyphRuleMatchesContract pins the one CONTRACT.md sentence about ASCII locales to both
// projections: Human on a live terminal draws bracketed rows, Plain always draws Unicode.
func TestASCIILocaleGlyphRuleMatchesContract(t *testing.T) {
	body, err := os.ReadFile("CONTRACT.md")
	if err != nil {
		t.Fatal(err)
	}
	flat := strings.Join(strings.Fields(string(body)), " ")
	if n := strings.Count(flat, asciiLocaleRuleSentence); n != 1 {
		t.Fatalf("CONTRACT.md states the ASCII-locale glyph rule %d times, want once: %q", n, asciiLocaleRuleSentence)
	}
	if n := strings.Count(flat, asciiLocaleRuleKeyword); n != 1 {
		t.Fatalf("CONTRACT.md mentions %q %d times, want once", asciiLocaleRuleKeyword, n)
	}

	human := ptyRowsAfterOneTask(t, Human)
	if !strings.Contains(human, asciiSucceededRow) || strings.Contains(human, unicodeSucceededRow) {
		t.Errorf("Human on a live terminal in a C locale must draw %q rows, not %q:\n%s", asciiSucceededRow, unicodeSucceededRow, human)
	}
	plain := ptyRowsAfterOneTask(t, Plain)
	if !strings.Contains(plain, unicodeSucceededRow) || strings.Contains(plain, asciiSucceededRow) {
		t.Errorf("Plain in a C locale must draw %q rows, not %q:\n%s", unicodeSucceededRow, asciiSucceededRow, plain)
	}
}
