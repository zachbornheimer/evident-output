package guards_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/catalog"
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
		sentence, start := sentenceAround(text, at)
		if !removedNote.MatchString(sentence) && !inReplacedPart(sentence, at-start) {
			unknown = append(unknown, name)
		}
	}
	for _, m := range evoIdentifier.FindAllStringSubmatchIndex(text, -1) {
		if name := text[m[2]:m[3]]; !c.names[name] {
			check("evo."+name, m[0])
		}
	}
	for _, m := range bareEffectName.FindAllStringSubmatchIndex(text, -1) {
		if name := text[m[2]:m[3]]; !c.names[name] {
			check(name, m[0])
		}
	}
	for _, m := range c.memberReference().FindAllStringSubmatchIndex(text, -1) {
		owner, member := text[m[2]:m[3]], text[m[4]:m[5]]
		if typeName, aliased := receiverTypes[owner]; aliased {
			owner = typeName
		}
		if !c.members[owner][member] {
			check(text[m[2]:m[3]]+"."+member, m[2])
		}
	}
	for _, hit := range unqualifiedRetiredNames(text) {
		check(hit.name, hit.at)
	}
	return unknown
}

// retiredNameHit is one bareRetiredNames word in text, and its offset.
type retiredNameHit struct {
	name string
	at   int
}

// unqualifiedRetiredNames are the bareRetiredNames words in text that are not
// qualified into another package (otherPackageQualified).
func unqualifiedRetiredNames(text string) []retiredNameHit {
	var hits []retiredNameHit
	for _, m := range bareRetiredName.FindAllStringSubmatchIndex(text, -1) {
		if !otherPackageQualified(text[:m[2]]) {
			hits = append(hits, retiredNameHit{name: text[m[2]:m[3]], at: m[2]})
		}
	}
	return hits
}

// otherPackageQualifiers precede a name that belongs to another package, not
// to the evo package the migration table retires: testkit's own Width and
// NoColor options, and the standard library's fmt.Print, fmt.Printf and
// fmt.Println, which the docs name when they say what to stop calling. A
// bare Print, Printf or Println stays forbidden: that is the removed
// evo.Print* spelling.
var otherPackageQualifiers = []string{"testkit.", "fmt."}

// otherPackageQualified reports whether before, the text up to a matched
// name, ends in a qualifier that puts the name in another package.
func otherPackageQualified(before string) bool {
	for _, qualifier := range otherPackageQualifiers {
		if strings.HasSuffix(before, qualifier) {
			return true
		}
	}
	return false
}

// bareEffectName matches an Effect-prefixed name written without the evo.
// qualifier (EffectSpec{Verb: EffectDelete}), the spelling 1.x taught for the
// verbs 2.0 exports as Create, Update and Delete.
var bareEffectName = regexp.MustCompile(`(?:^|[^\w.])(Effect[A-Z]\w*)`)

// sentenceBreak ends a sentence or clause: a period or semicolon, then a space.
var sentenceBreak = regexp.MustCompile(`[.;] `)

// sentenceAround is the sentence of text that holds offset at, and the offset
// where that sentence starts.
func sentenceAround(text string, at int) (sentence string, start int) {
	end := len(text)
	for _, b := range sentenceBreak.FindAllStringIndex(text, -1) {
		if b[1] <= at {
			start = b[1]
		} else {
			end = b[0]
			break
		}
	}
	return text[start:end], start
}

// inReplacedPart reports whether offset at (relative to sentence) falls in
// the code a "replace X with Y" suggestion tells the reader to remove: the
// X, which quotes the call site under review and so names removed API by
// definition. The Y is checked like any other text.
func inReplacedPart(sentence string, at int) bool {
	const verb = "replace "
	if len(sentence) < len(verb) || !strings.EqualFold(sentence[:len(verb)], verb) {
		return false
	}
	replacedEnd := strings.Index(sentence, " with ")
	return replacedEnd >= 0 && at < replacedEnd
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

// receiverTypes are the variable names the docs and rule examples give a
// Task, Group or Sequence, so task.Fail is checked as Task.Fail.
var receiverTypes = map[string]string{"task": "Task", "group": "Group", "seq": "Sequence"}

// memberReference matches Type.Member for each type with listed members, and
// for the receiver names in receiverTypes, not preceded by an identifier
// character (so ErrInvalidConfig.X, or a package path ending in a type name,
// never match).
func (c contractSurface) memberReference() *regexp.Regexp {
	types := make([]string, 0, len(c.members)+len(receiverTypes))
	for owner := range c.members {
		types = append(types, regexp.QuoteMeta(owner))
	}
	for receiver := range receiverTypes {
		types = append(types, receiver)
	}
	sort.Strings(types)
	return regexp.MustCompile(`(?:^|[^\w.])(` + strings.Join(types, "|") + `)\.([A-Z]\w*)`)
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
