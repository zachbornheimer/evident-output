package apisurface

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// GoldenRelPath and RequiredRelPath are the module-root-relative contract files.
const (
	GoldenRelPath   = "testdata/api_golden.txt"
	RequiredRelPath = "testdata/api_required.txt"
)

// RetiredNames are identifiers 1.0.0 deliberately removed (MainWith, Each)
// or never had (speculative names from earlier drafts/other libraries). A
// reappearance fails the contract even if testdata/api_golden.txt is
// rewritten to match, so retiring a name stays retired.
var RetiredNames = []string{
	"Task.Run", "Task.Go", "Task.Each",
	"DisplayGroup",
	"Group.Done",
	"Sequence.Fail",
	"TaskConfig",
	"MainWith",
	"IsLocked", "ReadLock", "WriteLock", "MultiLock",
	"TransactionBuilder", "ApplyPatch", "File.Patch",
	"Unlock", "Converge",
}

// Report is the four-bucket result of Check. Empty buckets mean that
// dimension passed. OK is true only when every bucket is empty.
type Report struct {
	Extra           []string
	Missing         []string
	RequiredMissing []string
	RetiredPresent  []string
}

// OK reports whether the live surface satisfies golden, required, and retired.
func (r Report) OK() bool {
	return len(r.Extra) == 0 && len(r.Missing) == 0 && len(r.RequiredMissing) == 0 && len(r.RetiredPresent) == 0
}

// String renders only the non-empty labeled sections.
func (r Report) String() string {
	var b strings.Builder
	writeSection(&b, "extra", r.Extra)
	writeSection(&b, "missing", r.Missing)
	writeSection(&b, "required-missing", r.RequiredMissing)
	writeSection(&b, "retired-present", r.RetiredPresent)
	return strings.TrimSuffix(b.String(), "\n")
}

// Error makes Report an error so the CLI can print labeled sections on exit 1.
func (r Report) Error() string {
	return r.String()
}

func writeSection(b *strings.Builder, label string, lines []string) {
	if len(lines) == 0 {
		return
	}
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	fmt.Fprintf(b, "%s:\n", label)
	for _, line := range lines {
		fmt.Fprintf(b, "  %s\n", line)
	}
}

// Check compares a live exported surface to golden, required, and retired
// lists. live and golden are full go/doc lines; required and retired are
// identifier tokens. Check does not read the filesystem.
func Check(live, golden, required, retired []string) Report {
	var r Report
	goldenSet := make(map[string]struct{}, len(golden))
	for _, line := range golden {
		goldenSet[line] = struct{}{}
	}
	liveSet := make(map[string]struct{}, len(live))
	for _, line := range live {
		liveSet[line] = struct{}{}
		if _, ok := goldenSet[line]; !ok {
			r.Extra = append(r.Extra, line)
		}
	}
	for _, line := range golden {
		if _, ok := liveSet[line]; !ok {
			r.Missing = append(r.Missing, line)
		}
	}
	for _, token := range required {
		token = strings.TrimSpace(token)
		if token == "" || strings.HasPrefix(token, "#") {
			continue
		}
		if !tokenPresent(live, token) {
			r.RequiredMissing = append(r.RequiredMissing, token)
		}
	}
	seen := make(map[string]struct{})
	for _, name := range retired {
		if name == "" {
			continue
		}
		for _, line := range live {
			if !retiredMatches(line, name) {
				continue
			}
			if _, ok := seen[name]; ok {
				break
			}
			seen[name] = struct{}{}
			r.RetiredPresent = append(r.RetiredPresent, name)
			break
		}
	}
	return r
}

// CheckDir walks dir (the module root), loads testdata/api_golden.txt and
// testdata/api_required.txt, and checks the live surface against both plus
// RetiredNames.
func CheckDir(dir string) (Report, error) {
	live, err := Walk(dir)
	if err != nil {
		return Report{}, err
	}
	golden, err := loadLines(filepath.Join(dir, GoldenRelPath))
	if err != nil {
		return Report{}, fmt.Errorf("apisurface: read %s: %w", GoldenRelPath, err)
	}
	required, err := loadRequired(filepath.Join(dir, RequiredRelPath))
	if err != nil {
		return Report{}, fmt.Errorf("apisurface: read %s: %w", RequiredRelPath, err)
	}
	return Check(live, golden, required, RetiredNames), nil
}

func loadLines(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.TrimRight(string(raw), "\n")
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}

func loadRequired(path string) ([]string, error) {
	lines, err := loadLines(path)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, nil
}

func tokenPresent(live []string, token string) bool {
	for _, line := range live {
		if lineContainsToken(line, token) {
			return true
		}
	}
	return false
}

func lineContainsToken(line, token string) bool {
	if strings.Contains(strings.TrimSpace(token), " ") {
		return strings.Contains(line, token)
	}
	return containsIdent(line, token)
}

func retiredMatches(line, name string) bool {
	if strings.Contains(line, name) {
		return true
	}
	typ, meth, ok := strings.Cut(name, ".")
	if ok {
		if strings.Contains(line, "func ("+typ+") "+meth) {
			return true
		}
	}
	if strings.Contains(name, ".") {
		return false
	}
	return containsIdent(line, name)
}

func containsIdent(s, ident string) bool {
	start := -1
	for i, r := range s {
		if isIdentRune(r, start < 0) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			if s[start:i] == ident {
				return true
			}
			start = -1
		}
	}
	return start >= 0 && s[start:] == ident
}

func isIdentRune(r rune, first bool) bool {
	if first {
		return unicode.IsLetter(r) || r == '_'
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
