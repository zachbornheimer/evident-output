package hillclimb

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
)

const minNounLength = 5

var wordPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9]*`)

// plainWords are common English words that carry no task identity.
var plainWords = map[string]bool{
	"about": true, "after": true, "before": true, "because": true, "being": true, "every": true,
	"first": true, "which": true, "while": true, "their": true, "there": true, "these": true,
	"those": true, "where": true, "would": true, "should": true, "write": true, "wait": true,
	"into": true, "that": true, "this": true, "then": true, "them": true, "with": true,
	"already": true, "again": true, "other": true, "still": true, "using": true, "doing": true,
}

// words lists each identifier lowercased, plus its camelCase parts, so a
// task word hidden inside an identifier is still found.
func words(text string) []string {
	var out []string
	for _, word := range wordPattern.FindAllString(text, -1) {
		out = append(out, strings.ToLower(word))
		out = append(out, camelParts(word)...)
	}
	return out
}

func camelParts(identifier string) []string {
	var parts []string
	start := 0
	runes := []rune(identifier)
	for i := 1; i < len(runes); i++ {
		if unicode.IsUpper(runes[i]) && unicode.IsLower(runes[i-1]) {
			parts = append(parts, strings.ToLower(string(runes[start:i])))
			start = i
		}
	}
	if start == 0 {
		return nil
	}
	return append(parts, strings.ToLower(string(runes[start:])))
}

// DistinctiveNouns are the words of the task prompts that the tuned
// vocabulary (the pre-edit docs) does not already use: domain words that name
// a task rather than teach the API.
func DistinctiveNouns(prompts []string, vocabulary string) []string {
	known := map[string]bool{}
	for _, word := range words(vocabulary) {
		known[word] = true
	}
	seen := map[string]bool{}
	var nouns []string
	for _, prompt := range prompts {
		for _, word := range words(prompt) {
			if len(word) < minNounLength || plainWords[word] || known[word] || seen[word] {
				continue
			}
			seen[word] = true
			nouns = append(nouns, word)
		}
	}
	slices.Sort(nouns)
	return nouns
}

// LintNouns returns the distinctive nouns present in the added text of an
// edit, i.e. task answers leaking into the tuned surface.
func LintNouns(added string, nouns []string) []string {
	present := map[string]bool{}
	for _, word := range words(added) {
		present[word] = true
	}
	var found []string
	for _, noun := range nouns {
		if present[noun] {
			found = append(found, noun)
		}
	}
	return found
}
