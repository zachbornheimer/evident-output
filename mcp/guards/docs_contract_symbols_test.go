package guards_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/catalog"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/rules"
)

// contractSymbolRow matches the first cell of a CONTRACT.md symbol table
// row: | `Name` | or | `Type.Member` |.
var contractSymbolRow = regexp.MustCompile("^\\|\\s*`([A-Za-z_]\\w*(?:\\.[A-Za-z_]\\w*)?)`\\s*\\|")

// contractSurface is the public surface CONTRACT.md lists: package-level
// names, and for each type the members it lists.
type contractSurface struct {
	names   map[string]bool
	members map[string]map[string]bool
}

func readContractSurface(t *testing.T, path string) contractSurface {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	surface := contractSurface{names: map[string]bool{}, members: map[string]map[string]bool{}}
	for line := range strings.SplitSeq(string(body), "\n") {
		row := contractSymbolRow.FindStringSubmatch(line)
		if row == nil || row[1] == "Symbol" {
			continue
		}
		owner, member, dotted := strings.Cut(row[1], ".")
		if !dotted {
			surface.names[owner] = true
			continue
		}
		if surface.members[owner] == nil {
			surface.members[owner] = map[string]bool{}
		}
		surface.members[owner][member] = true
	}
	if len(surface.names) == 0 {
		t.Fatalf("%s: no symbol tables found", path)
	}
	return surface
}

// unknownReferences are the evo.X and Type.Member references in text that
// the surface does not list, except those whose own sentence says the name
// was removed. Type.Member is checked for every type the contract lists
// members of.
func (c contractSurface) unknownReferences(text string) []string {
	var unknown []string
	check := func(name string, at int) {
		if !removedNote.MatchString(sentenceAround(text, at)) {
			unknown = append(unknown, name)
		}
	}
	for _, m := range evoIdentifier.FindAllStringSubmatchIndex(text, -1) {
		if name := text[m[2]:m[3]]; !c.names[name] {
			check("evo."+name, m[0])
		}
	}
	for _, m := range c.memberReference().FindAllStringSubmatchIndex(text, -1) {
		if owner, member := text[m[2]:m[3]], text[m[4]:m[5]]; !c.members[owner][member] {
			check(owner+"."+member, m[2])
		}
	}
	return unknown
}

// sentenceBreak ends a sentence or clause: a period or semicolon, then a space.
var sentenceBreak = regexp.MustCompile(`[.;] `)

// sentenceAround is the sentence of text that holds offset at.
func sentenceAround(text string, at int) string {
	start, end := 0, len(text)
	for _, b := range sentenceBreak.FindAllStringIndex(text, -1) {
		if b[1] <= at {
			start = b[1]
		} else {
			end = b[0]
			break
		}
	}
	return text[start:end]
}

// knows reports whether tag is a contract symbol: a package-level name, a
// Type.Member the contract lists, or the name of a member of a listed type.
func (c contractSurface) knows(tag string) bool {
	if c.names[tag] {
		return true
	}
	if owner, member, dotted := strings.Cut(tag, "."); dotted {
		return c.members[owner][member]
	}
	for _, members := range c.members {
		if members[tag] {
			return true
		}
	}
	return false
}

// memberReference matches Type.Member for each type with listed members,
// not preceded by an identifier character (so ErrInvalidConfig.X, or a
// package path ending in a type name, never match).
func (c contractSurface) memberReference() *regexp.Regexp {
	types := make([]string, 0, len(c.members))
	for owner := range c.members {
		types = append(types, regexp.QuoteMeta(owner))
	}
	sort.Strings(types)
	return regexp.MustCompile(`(?:^|[^\w.])(` + strings.Join(types, "|") + `)\.([A-Z]\w*)`)
}

// docCorpus is every text the MCP serves or ships as guidance, keyed by a
// readable origin: docs, skills, the embedded sections, the catalog guides,
// the rule texts and the review suggestions. A rule's BadCode and BadOutput
// are left out: they show the mistake on purpose.
func docCorpus(t *testing.T, root string) map[string]string {
	t.Helper()
	corpus := map[string]string{}
	for _, dir := range []string{"mcp/docs", "mcp/skills", "mcp/internal/agent/sections/embedded"} {
		walkFiles(t, filepath.Join(root, dir), ".md", func(rel, body string) { corpus[filepath.ToSlash(rel)] = body })
	}
	for _, guide := range catalog.All() {
		corpus["catalog guide "+guide.ID] = guide.Body
	}
	for _, rule := range rules.All() {
		if rule.MaxDialect != "" {
			continue // teaches the dialect below its ceiling, on purpose
		}
		for field, text := range map[string]string{
			"invariant": rule.Invariant, "why": rule.Why, "remediation": rule.Remediation,
			"good code": rule.GoodCode, "good output": rule.GoodOutput, "exceptions": strings.Join(rule.Exceptions, "\n"),
		} {
			corpus["rule "+rule.ID+" "+field] = text
		}
	}
	for _, dir := range []string{"mcp/internal/agent/review", "mcp/internal/agent/adopt"} {
		for site, text := range suggestionLiterals(t, filepath.Join(root, dir)) {
			corpus["suggestion "+site] = text
		}
	}
	return corpus
}

// walkFiles calls visit with the module-relative path and body of every file
// under dir that ends in suffix.
func walkFiles(t *testing.T, dir, suffix string, visit func(rel, body string)) {
	t.Helper()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, suffix) {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(moduleRoot(t), path)
		visit(rel, string(body))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
}

// suggestionLiterals are the string literals Go code in dir assigns to a
// Suggestion field, keyed by file:line.
func suggestionLiterals(t *testing.T, dir string) map[string]string {
	t.Helper()
	literals := map[string]string{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
			strings.Contains(filepath.ToSlash(path), "/testdata/") {
			return err
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if lit := suggestionValue(n); lit != nil {
				text, _ := strconv.Unquote(lit.Value)
				rel, _ := filepath.Rel(moduleRoot(t), path)
				literals[filepath.ToSlash(rel)+":"+strconv.Itoa(fset.Position(lit.Pos()).Line)] = text
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("parse %s: %v", dir, err)
	}
	return literals
}

// suggestionValue is the string literal n assigns to a field named
// Suggestion, in a composite literal key or an assignment, else nil.
func suggestionValue(n ast.Node) *ast.BasicLit {
	var value ast.Expr
	switch n := n.(type) {
	case *ast.KeyValueExpr:
		if key, ok := n.Key.(*ast.Ident); ok && key.Name == "Suggestion" {
			value = n.Value
		}
	case *ast.AssignStmt:
		if len(n.Lhs) == 1 && len(n.Rhs) == 1 {
			if sel, ok := n.Lhs[0].(*ast.SelectorExpr); ok && sel.Sel.Name == "Suggestion" {
				value = n.Rhs[0]
			}
		}
	}
	lit, _ := value.(*ast.BasicLit)
	if lit == nil || lit.Kind != token.STRING {
		return nil
	}
	return lit
}

// TestDocsNameOnlyContractSymbols fails on any evo.X, Config.X, Run.X,
// Task.X (and every other Type.Member of a type the contract lists members
// of) in the text the MCP teaches that CONTRACT.md's symbol tables do not
// list. TestDocsCarryNoStaleAPI knows the names retired so far; this guard
// needs no list, so a name 2.0 never had, or dropped without a note, cannot
// be taught. A sentence may name a symbol only to say it was removed. The
// catalog guides' capitalized Concepts tags must each be a contract symbol.
func TestDocsNameOnlyContractSymbols(t *testing.T) {
	root := moduleRoot(t)
	surface := readContractSurface(t, filepath.Join(root, "CONTRACT.md"))
	for _, guide := range catalog.All() {
		for _, tag := range guide.Concepts {
			if isCapitalized(tag) && !surface.knows(tag) {
				t.Errorf("catalog guide %s concept %q is not a CONTRACT.md symbol", guide.ID, tag)
			}
		}
	}
	for origin, text := range docCorpus(t, root) {
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			unknown := surface.unknownReferences(line)
			if len(unknown) == 0 {
				continue
			}
			t.Errorf("%s:%d names %s, which CONTRACT.md does not list:\n%s", origin, i+1, strings.Join(unknown, ", "), line)
		}
	}
}

// isCapitalized reports whether tag names a Go symbol, as opposed to a
// lowercase topic word such as "sanitize".
func isCapitalized(tag string) bool { return tag != "" && tag[0] >= 'A' && tag[0] <= 'Z' }

// removedNote marks a sentence that says a name was removed.
var removedNote = regexp.MustCompile(`(?i)\bremoved in\b`)
