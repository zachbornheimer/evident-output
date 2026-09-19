package engine

import (
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

type parsedPatchFile struct {
	oldPath string
	newPath string
	mode    fs.FileMode
	hunks   []patchHunk
	rename  bool
	deleted bool
	binary  bool
}

type patchHunk struct {
	oldStart, oldCount int
	newStart, newCount int
	lines              []patchLine
}

type patchLine struct {
	kind byte
	text string
	noNL bool
}

func parseUnifiedDiff(diff []byte) ([]parsedPatchFile, error) {
	lines := splitDiffLines(diff)
	var files []parsedPatchFile
	var cur *parsedPatchFile
	for i := 0; i < len(lines); {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flushParsedFile(&files, &cur)
			cur = &parsedPatchFile{}
			a, b, ok := gitDiffPaths(line)
			if ok && a != b && a != "/dev/null" && b != "/dev/null" {
				cur.rename = true
			}
			i++
		case strings.HasPrefix(line, "rename from "), strings.HasPrefix(line, "rename to "):
			ensureParsedFile(&cur)
			cur.rename = true
			i++
		case strings.HasPrefix(line, "copy from "), strings.HasPrefix(line, "copy to "):
			return nil, ErrPatchRename
		case line == "GIT binary patch", strings.HasPrefix(line, "Binary files "), strings.HasPrefix(line, "Binary file "):
			return nil, ErrPatchBinary
		case strings.HasPrefix(line, "deleted file mode "):
			ensureParsedFile(&cur)
			cur.deleted = true
			i++
		case strings.HasPrefix(line, "new file mode "):
			ensureParsedFile(&cur)
			cur.mode = parseGitFileMode(strings.TrimPrefix(line, "new file mode "))
			i++
		case strings.HasPrefix(line, "new mode "):
			ensureParsedFile(&cur)
			cur.mode = parseGitFileMode(strings.TrimPrefix(line, "new mode "))
			i++
		case strings.HasPrefix(line, "--- "):
			if cur != nil && cur.oldPath != "" {
				flushParsedFile(&files, &cur)
			}
			ensureParsedFile(&cur)
			cur.oldPath = parseDiffPath(line[4:])
			i++
		case strings.HasPrefix(line, "+++ "):
			ensureParsedFile(&cur)
			cur.newPath = parseDiffPath(line[4:])
			i++
		case strings.HasPrefix(line, "@@ "):
			ensureParsedFile(&cur)
			hunk, n, err := parseHunk(lines[i:])
			if err != nil {
				return nil, err
			}
			cur.hunks = append(cur.hunks, hunk)
			i += n
		default:
			i++
		}
	}
	flushParsedFile(&files, &cur)
	return files, nil
}

func (f parsedPatchFile) unsupported() error {
	if f.binary {
		return ErrPatchBinary
	}
	if f.deleted || f.newPath == "/dev/null" {
		return ErrPatchDelete
	}
	if f.rename {
		return ErrPatchRename
	}
	if f.oldPath != "" && f.newPath != "" && f.oldPath != "/dev/null" && f.newPath != "/dev/null" && f.oldPath != f.newPath {
		return ErrPatchRename
	}
	return nil
}

func ensureParsedFile(cur **parsedPatchFile) {
	if *cur == nil {
		*cur = &parsedPatchFile{}
	}
}

func flushParsedFile(files *[]parsedPatchFile, cur **parsedPatchFile) {
	if *cur == nil {
		return
	}
	if (*cur).oldPath != "" || (*cur).newPath != "" || len((*cur).hunks) > 0 {
		*files = append(*files, **cur)
	}
	*cur = nil
}

func splitDiffLines(diff []byte) []string {
	if len(diff) == 0 {
		return nil
	}
	s := string(diff)
	if s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	return strings.Split(s, "\n")
}

func parseDiffPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if tab := strings.IndexByte(raw, '\t'); tab >= 0 {
		raw = raw[:tab]
	}
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	}
	if raw == "/dev/null" {
		return raw
	}
	if strings.HasPrefix(raw, "a/") || strings.HasPrefix(raw, "b/") {
		return raw[2:]
	}
	return raw
}

func gitDiffPaths(line string) (a, b string, ok bool) {
	rest := strings.TrimPrefix(line, "diff --git ")
	parts := strings.Fields(rest)
	if len(parts) < 2 {
		return "", "", false
	}
	return parseDiffPath(parts[0]), parseDiffPath(parts[1]), true
}

func parseGitFileMode(s string) fs.FileMode {
	s = strings.TrimSpace(s)
	if len(s) < 3 {
		return 0
	}
	perm := s[len(s)-3:]
	n, err := strconv.ParseUint(perm, 8, 32)
	if err != nil {
		return 0
	}
	return fs.FileMode(n)
}

func parseHunk(lines []string) (patchHunk, int, error) {
	var h patchHunk
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "@@ ") {
		return h, 0, fmt.Errorf("%w: missing hunk header", ErrPatchMalformed)
	}
	oldStart, oldCount, newStart, newCount, err := parseHunkHeader(lines[0])
	if err != nil {
		return h, 0, err
	}
	h.oldStart, h.oldCount = oldStart, oldCount
	h.newStart, h.newCount = newStart, newCount
	oldLeft, newLeft := oldCount, newCount
	consumed := 1
	for consumed < len(lines) {
		line := lines[consumed]
		if strings.HasPrefix(line, "@@ ") || strings.HasPrefix(line, "diff --git ") || strings.HasPrefix(line, "--- ") {
			break
		}
		if strings.HasPrefix(line, "\\") {
			if len(h.lines) > 0 {
				h.lines[len(h.lines)-1].noNL = true
			}
			consumed++
			continue
		}
		if line == "" {
			line = " "
		}
		kind := line[0]
		if kind != ' ' && kind != '+' && kind != '-' {
			break
		}
		h.lines = append(h.lines, patchLine{kind: kind, text: line[1:]})
		consumed++
		if kind == ' ' || kind == '-' {
			oldLeft--
		}
		if kind == ' ' || kind == '+' {
			newLeft--
		}
	}
	if oldLeft != 0 || newLeft != 0 {
		return h, consumed, fmt.Errorf("%w: hunk line counts do not match header", ErrPatchMalformed)
	}
	return h, consumed, nil
}

func parseHunkHeader(line string) (oldStart, oldCount, newStart, newCount int, err error) {
	rest := strings.TrimPrefix(line, "@@ ")
	if !strings.HasPrefix(rest, "-") {
		return 0, 0, 0, 0, fmt.Errorf("%w: malformed hunk header", ErrPatchMalformed)
	}
	rest = rest[1:]
	plus := strings.IndexByte(rest, '+')
	if plus < 0 {
		return 0, 0, 0, 0, fmt.Errorf("%w: malformed hunk header", ErrPatchMalformed)
	}
	oldPart := strings.TrimSpace(rest[:plus])
	newPart := strings.TrimSpace(rest[plus+1:])
	if at := strings.Index(newPart, "@@"); at >= 0 {
		newPart = strings.TrimSpace(newPart[:at])
	}
	oldStart, oldCount, err = parseHunkRange(oldPart)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	newStart, newCount, err = parseHunkRange(newPart)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return oldStart, oldCount, newStart, newCount, nil
}

func parseHunkRange(part string) (start, count int, err error) {
	startStr, countStr, hasCount := strings.Cut(part, ",")
	start, err = strconv.Atoi(startStr)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: malformed hunk range", ErrPatchMalformed)
	}
	if !hasCount {
		return start, 1, nil
	}
	count, err = strconv.Atoi(countStr)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: malformed hunk range", ErrPatchMalformed)
	}
	return start, count, nil
}
