package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
	"github.com/zachbornheimer/evident-output/internal/agent/rules"
)

// evoDefineBody wraps body as the Define callback of one Task, with ctx,
// errors, and a diff in scope: where 1.2 File/Tree/Exec/Patch calls live.
func evoDefineBody(body string) string {
	return "package p\nimport (\n\t\"context\"\n\t\"errors\"\n\tevo \"github.com/zachbornheimer/evident-output\"\n)\n" +
		"var _ = errors.Is\n" +
		"func f(task *evo.TaskHandle, diff []byte) {\n\ttask.Define(func(ctx context.Context) error {\n" +
		indentBody(body) + "\t})\n}\n"
}

// indentBody indents each line of a fixture body into the Define callback.
func indentBody(body string) string {
	lines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "\t\t" + l
	}
	return strings.Join(lines, "\n") + "\n"
}

// addFileTreeFixtures covers the exports ZYS-1382 retired in 1.2: the
// spec+function shapes became plain File/Tree/Exec structs, FSPath became
// Task Basis, and Patch applies deletes and renames directly.
func addFileTreeFixtures(m map[string]migrationFixture) {
	for name, fx := range map[string]migrationFixture{
		"FileSpec": {
			dirty: `return evo.File(ctx, evo.FileSpec{Path: "a.txt", Contents: []byte("x")})`,
			clean: `return evo.File{Path: "a.txt", Content: evo.Bytes("x")}.Write(ctx)`,
		},
		"ExecSpec": {
			dirty: "_, err := evo.Exec(ctx, evo.ExecSpec{Executable: \"go\", Args: []string{\"build\"}})\nreturn err",
			clean: "_, err := evo.Exec{Path: \"go\", Args: []string{\"build\"}}.Run(ctx)\nreturn err",
		},
		"FSPath": {
			dirty: "_ = evo.FSPath(\"go.mod\")\nreturn nil",
			clean: "_, err := evo.File{Path: \"go.mod\"}.Read(ctx)\nreturn err",
		},
		"FileSet": {
			dirty: "var set evo.FileSet\n_ = set\nreturn evo.Patch(ctx, diff)",
			clean: "return evo.Patch(ctx, diff)",
		},
		"Files": {
			dirty: "return evo.Files(ctx, nil)",
			clean: "return evo.Patch(ctx, diff)",
		},
		"ErrStaleBasis":                   sentinelRename("evo.Patch(ctx, diff)", "ErrStaleBasis", "ErrPatchStale"),
		"ErrFileSpecMissingPath":          sentinelRename("(evo.File{}).Write(ctx)", "ErrFileSpecMissingPath", "ErrPathMissing"),
		"ErrFileUnmanagedContentsMissing": sentinelRename(`(evo.File{Path: "a.txt"}).Write(ctx)`, "ErrFileUnmanagedContentsMissing", "ErrContentMissing"),
		"ErrExecSpecMissingExecutable": {
			dirty: "if _, err := (evo.Exec{}).Run(ctx); errors.Is(err, evo.ErrExecSpecMissingExecutable) {\n\treturn err\n}\nreturn nil",
			clean: "if _, err := (evo.Exec{}).Run(ctx); errors.Is(err, evo.ErrExecPathMissing) {\n\treturn err\n}\nreturn nil",
		},
		"ErrPatchDeleteUnsupported": {
			dirty: "if err := evo.Patch(ctx, diff); !errors.Is(err, evo.ErrPatchDeleteUnsupported) {\n\treturn err\n}\nreturn nil",
			clean: "return evo.Patch(ctx, diff)",
		},
		"ErrPatchRenameUnsupported": {
			dirty: "if err := evo.Patch(ctx, diff); !errors.Is(err, evo.ErrPatchRenameUnsupported) {\n\treturn err\n}\nreturn nil",
			clean: "return evo.Patch(ctx, diff)",
		},
	} {
		m[name] = migrationFixture{dirty: evoDefineBody(fx.dirty), clean: evoDefineBody(fx.clean)}
	}
}

// sentinelRename is a fixture whose only change is a one-to-one sentinel
// rename in an errors.Is check on call's error.
func sentinelRename(call, old, next string) migrationFixture {
	body := func(sentinel string) string {
		return "if err := " + call + "; errors.Is(err, evo." + sentinel + ") {\n\treturn err\n}\nreturn nil"
	}
	return migrationFixture{dirty: body(old), clean: body(next)}
}

// retiredIn1_2 reports whether the retired table removed name in 1.2.
// 1.2 names are migrations (multi-line restructures, sentinel renames), so
// they have their own test; every other removed name keeps the strict
// single-replace check of the 1.1 test.
func retiredIn1_2(name string) bool {
	for _, s := range rules.RetiredSymbols() {
		if s.Contract == name && s.RemovedIn == rules.RetiredRelease1_2 {
			return true
		}
	}
	return false
}

// TestMigration1_2EveryRetiredNameHasDirtyCleanFixture requires each name
// the retired table removed in 1.2 to have a dirty fixture that review flags
// with its migration (a Suggestion or the table's Replacement in the Message), and a clean fixture that passes.
func TestMigration1_2EveryRetiredNameHasDirtyCleanFixture(t *testing.T) {
	fixtures := map[string]migrationFixture{}
	addFileTreeFixtures(fixtures)
	var retired []string
	replacement := map[string]string{}
	for _, s := range rules.RetiredSymbols() {
		if s.RemovedIn != rules.RetiredRelease1_2 {
			continue
		}
		replacement[s.Contract] = s.Replacement
		retired = append(retired, s.Contract)
		if _, ok := fixtures[s.Contract]; !ok {
			t.Errorf("missing 1.2 fixture for retired name %s", s.Contract)
		}
	}
	if len(retired) == 0 {
		t.Fatal("retired table holds no names removed in 1.2")
	}
	for name := range fixtures {
		if !retiredIn1_2(name) {
			t.Errorf("1.2 fixture %s is not retired in 1.2 in the retired table", name)
		}
	}
	for _, name := range retired {
		fx, ok := fixtures[name]
		if !ok {
			continue
		}
		t.Run(name, func(t *testing.T) {
			dirty := review.GoSource(name+".go", fx.dirty)
			hit, ok := findingAbout(migrationFindings(dirty), name)
			if !ok {
				t.Fatalf("dirty %s: no migration finding about it; got %+v", name, dirty.Findings)
			}
			if guidance := hit.Suggestion + hit.Message; !strings.Contains(guidance, replacement[name]) {
				t.Fatalf("dirty %s: finding does not carry the migration %q: %+v", name, replacement[name], hit)
			}
			if _, ok := rules.Explain(hit.RuleID); !ok {
				t.Fatalf("rules.Explain(%q) failed", hit.RuleID)
			}
			clean := review.GoSource(name+".go", fx.clean)
			if len(clean.Findings) != 0 || clean.RecheckRequired {
				t.Fatalf("clean %s is dirty: recheck=%v findings=%+v", name, clean.RecheckRequired, clean.Findings)
			}
		})
	}
}
