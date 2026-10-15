package guards_test

import (
	"regexp"
	"strings"
)

// backtickSpan matches one inline code span on a line.
var backtickSpan = regexp.MustCompile("`([^`]+)`")

// dottedIdentifier matches an identifier chain: Run, Task.Doing, evo.Main.
var dottedIdentifier = regexp.MustCompile(`[A-Za-z_]\w*(?:\.[A-Za-z_]\w*)*`)

// exportedGoName matches a Go exported identifier in CamelCase. ALL-CAPS
// words (GOPATH, HOME) and file names are not evo symbols.
var exportedGoName = regexp.MustCompile(`^[A-Z]\w*[a-z]\w*$`)

// chain0 is the first element of a dotted chain, or its second when the chain
// starts with the evo package qualifier or a lowercase receiver.
func chain0(chain string) string {
	parts := strings.Split(chain, ".")
	if len(parts) > 1 && (parts[0] == "evo" || !isCapitalized(parts[0])) {
		return parts[1]
	}
	return parts[0]
}

// nonEvoIdentifiers are capitalized words the docs put in backticks that
// name no evo symbol: wire formats, output profiles, environment-adjacent
// terms and Go's own predeclared or standard names. The set is closed and
// small on purpose; a word the docs need that is not here is either a 2.0
// symbol (add it to CONTRACT.md) or a 1.x name to remove.
var nonEvoIdentifiers = map[string]bool{
	"JSON": true, "JSONL": true, "Plain": true, "TTY": true, "ANSI": true,
	"NDJSON": true, "NO_COLOR": true, "CI": true, "TERM": true, "SIGINT": true,
	"SIGTERM": true, "EOF": true, "Unreleased": true,
	"Python": true, "Go": true, // language names inside example Task names
	"Type": true, // the placeholder in CONTRACT.md's notation: Type.Name
}

// goTestName matches a Go test function, which the docs cite as proof.
var goTestName = regexp.MustCompile(`^Test[A-Z]`)

// backtickedUnknowns are the identifiers inside inline code spans of text
// that resolve to no 2.0 symbol: a capitalized word that CONTRACT.md does not
// list, or a Type.Member the contract does not list for that type. A span's
// package-qualified names (io.Writer, errors.Is, os.Exit) belong to other
// packages and are skipped; evo.X and receiver.Member are resolved against
// the contract. Each hit carries its offset in text so the caller can read
// the sentence around it.
func (c contractSurface) backtickedUnknowns(text string) []retiredNameHit {
	var hits []retiredNameHit
	for _, span := range backtickSpan.FindAllStringSubmatchIndex(text, -1) {
		body := text[span[2]:span[3]]
		if strings.ContainsAny(body, "/\"$") {
			continue // a path, URL or shell command, not Go
		}
		for _, chain := range dottedIdentifier.FindAllStringIndex(body, -1) {
			name := body[chain[0]:chain[1]]
			if !exportedGoName.MatchString(chain0(name)) || c.resolvesChain(name) {
				continue
			}
			hits = append(hits, retiredNameHit{name: name, at: span[2] + chain[0]})
		}
	}
	return hits
}

// resolvesChain reports whether the dotted identifier chain names a 2.0
// symbol, or names nothing of evo's at all.
func (c contractSurface) resolvesChain(chain string) bool {
	parts := strings.Split(chain, ".")
	if parts[0] == "evo" {
		parts = parts[1:]
		if len(parts) == 0 {
			return true
		}
	} else if owner, receiver := receiverTypes[parts[0]]; receiver {
		parts[0] = owner
	} else if !isCapitalized(parts[0]) {
		return true // a local variable or another package's name
	}
	if nonEvoIdentifiers[parts[0]] || goTestName.MatchString(parts[0]) || c.names[parts[0]] && len(parts) == 1 {
		return true
	}
	if len(parts) == 1 {
		return c.knows(parts[0])
	}
	if c.members[parts[0]][parts[1]] {
		return true
	}
	return c.names[parts[0]] && !c.hasMembers(parts[0])
}

// hasMembers reports whether the contract lists any member of owner.
func (c contractSurface) hasMembers(owner string) bool { return len(c.members[owner]) > 0 }
