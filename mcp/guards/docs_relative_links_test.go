package guards_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

// docsDir holds the MCP docs; the embedded sections are generated copies of
// them, so their relative links resolve against this directory, not their own.
const docsDir = "mcp/docs"

// embeddedSectionsDir holds the generated copies the MCP serves.
const embeddedSectionsDir = "mcp/internal/agent/sections/embedded"

// markdownLink matches [text](target); the target ends at whitespace or ")".
var markdownLink = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)[^)]*\)`)

// markdownHeading matches an ATX heading line.
var markdownHeading = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*#*\s*$`)

// linkRef is one relative link: its target path, optional fragment, and line.
type linkRef struct {
	path, fragment string
	line           int
}

// String is the link as written: path, then #fragment when there is one.
func (l linkRef) String() string {
	if l.fragment == "" {
		return l.path
	}
	return l.path + "#" + l.fragment
}

// relativeLinks are the links in a markdown body that point into the
// repository: not a URL, not a mailto, outside fenced code.
func relativeLinks(body string) []linkRef {
	var refs []linkRef
	fenced := false
	for i, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		for _, m := range markdownLink.FindAllStringSubmatch(line, -1) {
			target := m[1]
			if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			path, fragment, _ := strings.Cut(target, "#")
			refs = append(refs, linkRef{path: path, fragment: fragment, line: i + 1})
		}
	}
	return refs
}

// headingSlug is the anchor GitHub gives a heading: lowercase, spaces to
// hyphens, punctuation dropped.
func headingSlug(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.ReplaceAll(heading, "`", "")) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// hasAnchor reports whether the markdown body has a heading whose slug is
// fragment.
func hasAnchor(body, fragment string) bool {
	fenced := false
	for line := range strings.SplitSeq(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
		}
		if m := markdownHeading.FindStringSubmatch(line); !fenced && m != nil && headingSlug(m[1]) == fragment {
			return true
		}
	}
	return false
}

// brokenLink explains why link, written in a file of baseDir, does not
// resolve, or returns "" when it does.
func brokenLink(root, baseDir string, self string, link linkRef) string {
	target := self
	if link.path != "" {
		target = filepath.Join(root, baseDir, filepath.FromSlash(link.path))
	}
	info, err := os.Stat(target)
	if err != nil {
		return "no such file"
	}
	if link.fragment == "" || info.IsDir() || !strings.HasSuffix(target, ".md") {
		return ""
	}
	body, err := os.ReadFile(target)
	if err != nil || !hasAnchor(string(body), link.fragment) {
		return "no heading for #" + link.fragment
	}
	return ""
}

// TestDocsRelativeLinksResolve fails on any relative link in mcp/docs, or in
// its embedded copies, whose file or heading does not exist. The MCP serves
// these docs to agents that cannot ask what a dead link meant.
func TestDocsRelativeLinksResolve(t *testing.T) {
	root := moduleRoot(t)
	for _, dir := range []string{docsDir, embeddedSectionsDir} {
		walkFiles(t, filepath.Join(root, dir), ".md", func(rel, body string) {
			self := filepath.Join(root, rel)
			if dir == embeddedSectionsDir {
				self = "" // an anchor-only link in a copy has no file of its own
			}
			for _, link := range relativeLinks(body) {
				if link.path == "" && self == "" {
					continue
				}
				if why := brokenLink(root, docsDir, self, link); why != "" {
					t.Errorf("%s:%d links %s: %s", rel, link.line, link.String(), why)
				}
			}
		})
	}
}

// TestRelativeLinkSlugs pins the heading-to-anchor rule the link guard uses.
func TestRelativeLinkSlugs(t *testing.T) {
	for heading, want := range map[string]string{
		"Construction, config, lifecycle": "construction-config-lifecycle",
		"`evo.Main` and friends":          "evomain-and-friends",
		"One check Task, many Problems":   "one-check-task-many-problems",
	} {
		if got := headingSlug(heading); got != want {
			t.Errorf("headingSlug(%q) = %q, want %q", heading, got, want)
		}
	}
}
