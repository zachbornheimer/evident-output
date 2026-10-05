package docexamples

import (
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"regexp"
	"strings"
)

// A rule snippet (an MCP rule's GoodCode) is a fragment, not a program: it
// calls evo against conventional receivers (task, out, ctx, ...) and
// against the application's own symbols (check, paths, cobra, ...), which
// no snippet declares. TypeCheckSnippet type-checks it against the real evo
// package with the conventional receivers declared at their evo types, so
// every evo call is checked for real — method names, argument counts and
// types, callback signatures — while an application symbol the fragment
// only names in passing is left undeclared and ignored.

// snippetFileSet and evoImporter are shared by every TypeCheckSnippet call: the source
// importer caches each package it type-checks, so evo is loaded once per
// test binary instead of once per snippet.
var (
	snippetFileSet = token.NewFileSet()
	evoImporter    = importer.ForCompiler(snippetFileSet, "source", nil)
)

// snippetImports is every standard-library package a rule snippet reaches
// for; unused ones are tolerated (see ignoredSnippetError).
var snippetImports = []string{
	"context", "encoding/json", "errors", "fmt", "os", "os/exec",
	"os/signal", "path/filepath", "strconv", "strings", "syscall",
	"github.com/zachbornheimer/evident-output",
}

// snippetReceivers declares the conventional names rule snippets use for
// evo values, at the types those names always denote.
const snippetReceivers = `
var (
	out *evo.Output
	task, t, it, configTask, cacheWarmTask, consumer, producer *evo.TaskHandle
	group, prune, worktrees *evo.GroupHandle
	ctx context.Context
	err error
	cmd *exec.Cmd
	reason evo.TaxonomyReason
	n int
)
`

// ignoredSnippetError reports whether msg is a consequence of the snippet
// being a fragment rather than an evo API mistake: an undeclared
// application symbol (a bare identifier, never "evo.X"), a value the
// fragment computes but never uses, or a function body the fragment
// leaves to the reader.
var ignoredSnippetError = regexp.MustCompile(
	`^undefined: [A-Za-z_][A-Za-z0-9_]*$|declared and not used|is not used|imported (as \w+ )?and not used|missing return`)

// TypeCheckSnippet type-checks src (a whole file's declarations, or a run
// of statements) against evo. It returns the evo API errors — never the
// fragment noise ignoredSnippetError filters — or a parse error when src
// is not Go at all.
func TypeCheckSnippet(src string) ([]error, error) {
	file, err := parseSnippet(snippetFileSet, src)
	if err != nil {
		return nil, err
	}
	receivers, err := parser.ParseFile(snippetFileSet, "receivers.go", snippetHeader()+snippetReceivers, 0)
	if err != nil {
		return nil, fmt.Errorf("parse snippet receivers: %w", err)
	}
	var apiErrors []error
	conf := types.Config{
		Importer: evoImporter,
		Error: func(err error) {
			var typeErr types.Error
			if errors.As(err, &typeErr) && ignoredSnippetError.MatchString(typeErr.Msg) {
				return
			}
			apiErrors = append(apiErrors, err)
		},
	}
	_, _ = conf.Check("snippet", snippetFileSet, []*ast.File{receivers, file}, nil)
	return apiErrors, nil
}

// parseSnippet parses src as a file of declarations, or else as statements
// inside a function that returns error (snippets "return err" freely).
func parseSnippet(fset *token.FileSet, src string) (*ast.File, error) {
	if file, err := parser.ParseFile(fset, "snippet.go", snippetHeader()+src, 0); err == nil {
		return file, nil
	}
	body := snippetHeader() + "func _() error {\n" + src + "\nreturn nil\n}\n"
	file, err := parser.ParseFile(fset, "snippet.go", body, 0)
	if err != nil {
		return nil, fmt.Errorf("parse snippet as declarations or statements: %w", err)
	}
	return file, nil
}

func snippetHeader() string {
	var b strings.Builder
	b.WriteString("package snippet\n\nimport (\n")
	for _, path := range snippetImports {
		fmt.Fprintf(&b, "\t%q\n", path)
	}
	b.WriteString(")\n\n")
	return b.String()
}
