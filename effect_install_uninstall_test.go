package evo

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
)

// TestEffectInstall_PlannedAndChangedTense proves EffectInstall and
// EffectUninstall are compile-tested public constants (ZYS-974) whose
// [planned] row keeps the imperative verb and whose [changed] row conjugates
// past tense ("installed"/"uninstalled"), matching every other EffectVerb.
func TestEffectInstall_PlannedAndChangedTense(t *testing.T) {
	t.Parallel()

	t.Run("planned keeps the imperative", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		out := initRun(Config{DryRun: true, Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
		out.Task("packages").Define(effectOf(Verb(change.EffectInstall), "package", 3))
		if err := out.finish(); err != nil {
			t.Fatal(err)
		}
		rendered := buf.String()
		if !strings.Contains(rendered, "install 3 packages") {
			t.Fatalf("want planned imperative %q in:\n%s", "install 3 packages", rendered)
		}
	})

	t.Run("changed conjugates past tense", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
		out.Task("packages").Define(effectOf(Verb(change.EffectInstall), "package", 3))
		if err := out.finish(); err != nil {
			t.Fatal(err)
		}
		rendered := buf.String()
		if !strings.Contains(rendered, "installed 3 packages") {
			t.Fatalf("want changed past tense %q in:\n%s", "installed 3 packages", rendered)
		}
	})

	t.Run("uninstall changed conjugates past tense", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
		out.Task("packages").Define(effectOf(Verb(change.EffectUninstall), "package", 1))
		if err := out.finish(); err != nil {
			t.Fatal(err)
		}
		rendered := buf.String()
		if !strings.Contains(rendered, "uninstalled 1 package\n") {
			t.Fatalf("want changed past tense singular in:\n%s", rendered)
		}
	})

	t.Run("JSON structured verb data carries the same tense", func(t *testing.T) {
		t.Parallel()
		out := initRun(Config{Output: Plain})
		out.Task("packages").Define(effectOf(Verb(change.EffectInstall), "package", 3))
		if err := out.finish(); err != nil {
			t.Fatal(err)
		}
		doc := out.decodedRunDocument(t)
		if len(doc.Data.Effects) != 1 {
			t.Fatalf("want one effect, got %+v", doc.Data.Effects)
		}
		got := doc.Data.Effects[0]
		if got.Verb != "installed" {
			t.Fatalf("want JSON verb %q, got %q", "installed", got.Verb)
		}
		if got.Quantity == nil || *got.Quantity != 3 {
			t.Fatalf("want quantity 3, got %+v", got.Quantity)
		}
	})
}
