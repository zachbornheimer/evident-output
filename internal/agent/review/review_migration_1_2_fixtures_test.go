package review_test

import "strings"

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
