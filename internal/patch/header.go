package patch

import (
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

// devNull is the path a diff names for the absent side of a creation or
// deletion.
const devNull = "/dev/null"

// Git's file-type prefixes on a six-digit octal mode.
const (
	gitRegularFile = "100"
	gitSymlink     = "120"
	gitSubmodule   = "160"
)

// header is one file section as read, before it is judged representable.
type header struct {
	gitPath  string // from "diff --git", when both sides name one path
	hasPaths bool   // a "---"/"+++" pair was present
	oldPath  string
	newPath  string
	create   bool
	delete   bool
	rename   bool
	copied   bool
	from     string // "rename from"
	to       string // "rename to"
	binary   bool
	mode     string // raw octal from "new file mode"/"new mode"
	hunks    []Hunk
}

// section reads the file section starting at the current line, if one
// starts there.
func (p *parser) section() (header, bool, error) {
	line := trimCR(p.peek())
	switch {
	case strings.HasPrefix(line, "diff --git "):
		p.next()
		h := header{gitPath: gitHeaderPath(strings.TrimPrefix(line, "diff --git "))}
		return h, true, p.gitExtended(&h)
	case isFileHeaderPair(line, p.peekAt(1)):
		var h header
		return h, true, p.pathsAndHunks(&h)
	case strings.HasPrefix(line, "Binary files ") && strings.HasSuffix(line, " differ"):
		return header{}, false, fmt.Errorf("%w: %s", ErrBinaryUnsupported, line)
	}
	return header{}, false, nil
}

func isFileHeaderPair(first, second string) bool {
	return strings.HasPrefix(first, "--- ") && strings.HasPrefix(trimCR(second), "+++ ")
}

// gitExtended reads git's extended header lines up to the "---"/"+++"
// pair, or to the first line that is not one (the next section).
func (p *parser) gitExtended(h *header) error {
	for p.more() {
		line := trimCR(p.peek())
		if isFileHeaderPair(line, p.peekAt(1)) {
			return p.pathsAndHunks(h)
		}
		if !h.extendedLine(line) {
			return nil
		}
		p.next()
	}
	return nil
}

// extendedLine records one git extended header line, reporting whether
// line was one.
func (h *header) extendedLine(line string) bool {
	switch {
	case strings.HasPrefix(line, "new file mode "):
		h.create, h.mode = true, strings.TrimPrefix(line, "new file mode ")
	case strings.HasPrefix(line, "deleted file mode "):
		h.delete = true
	case strings.HasPrefix(line, "new mode "):
		h.mode = strings.TrimPrefix(line, "new mode ")
	case strings.HasPrefix(line, "rename from "):
		h.rename, h.from = true, headerPath(strings.TrimPrefix(line, "rename from "))
	case strings.HasPrefix(line, "rename to "):
		h.rename, h.to = true, headerPath(strings.TrimPrefix(line, "rename to "))
	case hasAnyPrefix(line, "copy from ", "copy to "):
		h.rename, h.copied = true, true
	case strings.HasPrefix(line, "Binary files "), line == "GIT binary patch":
		h.binary = true
	case hasAnyPrefix(line, "old mode ", "index ", "similarity index ", "dissimilarity index "):
	default:
		return false
	}
	return true
}

// pathsAndHunks reads a "---"/"+++" pair and the hunks that follow it.
func (p *parser) pathsAndHunks(h *header) error {
	h.hasPaths = true
	h.oldPath = headerPath(strings.TrimPrefix(trimCR(p.next()), "--- "))
	h.newPath = headerPath(strings.TrimPrefix(trimCR(p.next()), "+++ "))
	for p.more() && strings.HasPrefix(p.peek(), "@@ ") {
		hunk, err := p.hunk()
		if err != nil {
			return fmt.Errorf("%s: %w", h.newPath, err)
		}
		if n := len(h.hunks); n > 0 && hunk.OldStart <= h.hunks[n-1].OldStart {
			return fmt.Errorf("%w: hunks for %s are out of order", ErrMalformed, h.newPath)
		}
		h.hunks = append(h.hunks, hunk)
	}
	return nil
}

// file judges h representable as a desired regular-file state.
func (h header) file() (File, error) {
	oldPath, newPath := h.sidePaths()
	switch {
	case h.delete || newPath == devNull:
		return File{}, fmt.Errorf("%w: %s", ErrDeleteUnsupported, oldPath)
	case h.rename || (h.hasPaths && oldPath != devNull && oldPath != newPath):
		return File{}, fmt.Errorf("%w: %s -> %s", ErrRenameUnsupported, oldPath, newPath)
	case h.binary:
		return File{}, fmt.Errorf("%w: %s", ErrBinaryUnsupported, newPath)
	}
	target, err := workspacePath(newPath)
	if err != nil {
		return File{}, err
	}
	mode, err := regularFileMode(h.mode)
	if err != nil {
		return File{}, fmt.Errorf("%s: %w", target, err)
	}
	return File{Path: target, Create: h.create || oldPath == devNull, Mode: mode, Hunks: h.hunks}, nil
}

// sidePaths returns the old and new paths with git's "a/"/"b/" prefixes
// removed when both present sides carry them.
func (h header) sidePaths() (oldPath, newPath string) {
	if !h.hasPaths {
		return h.gitPath, h.gitPath
	}
	oldPath, newPath = h.oldPath, h.newPath
	if prefixedOrNull(oldPath, "a/") && prefixedOrNull(newPath, "b/") {
		oldPath, newPath = strings.TrimPrefix(oldPath, "a/"), strings.TrimPrefix(newPath, "b/")
	}
	return oldPath, newPath
}

func prefixedOrNull(p, prefix string) bool { return p == devNull || strings.HasPrefix(p, prefix) }

// regularFileMode reads a git octal mode ("100755") as permission bits.
// "" means the patch leaves the mode alone. Symlink and submodule modes
// cannot reduce to a regular file's state.
func regularFileMode(raw string) (fs.FileMode, error) {
	if raw == "" {
		return 0, nil
	}
	if len(raw) != 6 {
		return 0, fmt.Errorf("%w: file mode %q", ErrMalformed, raw)
	}
	switch raw[:3] {
	case gitRegularFile:
	case gitSymlink:
		return 0, fmt.Errorf("%w: symlink", ErrUnsupported)
	case gitSubmodule:
		return 0, fmt.Errorf("%w: submodule", ErrUnsupported)
	default:
		return 0, fmt.Errorf("%w: file mode %q", ErrMalformed, raw)
	}
	perm, err := strconv.ParseUint(raw[3:], 8, 32)
	if err != nil {
		return 0, fmt.Errorf("%w: file mode %q", ErrMalformed, raw)
	}
	return fs.FileMode(perm), nil
}

// gitHeaderPath reads "a/X b/X" (or "X X") as X; any other shape, such as
// a rename's two paths, yields "".
func gitHeaderPath(s string) string {
	if len(s)%2 == 0 {
		return ""
	}
	half := len(s) / 2
	if s[half] != ' ' {
		return ""
	}
	left, right := s[:half], s[half+1:]
	if strings.HasPrefix(left, "a/") && strings.HasPrefix(right, "b/") {
		left, right = left[2:], right[2:]
	}
	if left != right {
		return ""
	}
	return left
}

// headerPath reads a "---"/"+++" path: a Go-quoted path as git writes one
// with special characters, or a bare path up to an optional tab-separated
// timestamp.
func headerPath(s string) string {
	if strings.HasPrefix(s, `"`) {
		if quoted, err := strconv.QuotedPrefix(s); err == nil {
			if unquoted, err := strconv.Unquote(quoted); err == nil {
				return unquoted
			}
		}
	}
	path, _, _ := strings.Cut(s, "\t")
	return path
}

func trimCR(s string) string { return strings.TrimSuffix(s, "\r") }

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
