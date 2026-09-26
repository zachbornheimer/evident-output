package evo_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
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
		out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true, DryRun: true})
		out.Task("packages").Define(effectOf(evo.EffectInstall, "package", 3))
		if err := out.Finish(); err != nil {
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
		out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
		out.Task("packages").Define(effectOf(evo.EffectInstall, "package", 3))
		if err := out.Finish(); err != nil {
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
		out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
		out.Task("packages").Define(effectOf(evo.EffectUninstall, "package", 1))
		if err := out.Finish(); err != nil {
			t.Fatal(err)
		}
		rendered := buf.String()
		if !strings.Contains(rendered, "uninstalled 1 package\n") {
			t.Fatalf("want changed past tense singular in:\n%s", rendered)
		}
	})

	t.Run("JSON structured verb data carries the same tense", func(t *testing.T) {
		t.Parallel()
		out := evo.Init(evo.Config{Isolated: true, Plain: true})
		out.Task("packages").Define(effectOf(evo.EffectInstall, "package", 3))
		if err := out.Finish(); err != nil {
			t.Fatal(err)
		}
		data, err := evo.EncodeJSONForTest(out.Snapshot())
		if err != nil {
			t.Fatal(err)
		}
		var doc evo.JSONDocumentForTest
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		if len(doc.Changes) != 1 || len(doc.Changes[0].Records) != 1 {
			t.Fatalf("want one Changes record, got %+v", doc.Changes)
		}
		got := doc.Changes[0].Records[0]
		if got.Verb != "installed" {
			t.Fatalf("want JSON verb %q, got %q", "installed", got.Verb)
		}
		if got.Quantity == nil || *got.Quantity != 3 {
			t.Fatalf("want quantity 3, got %+v", got.Quantity)
		}
	})
}
