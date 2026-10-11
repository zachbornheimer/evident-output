// This file reads a unified text diff (plain `diff -u` or extended headers)
// into per-file edits, purely: it never touches the filesystem, and forms an
// edit cannot express faithfully are rejected, never approximated.

package change

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
)

// Errors every Parse/Apply failure wraps.
var (
	// ErrPatchMalformed is a diff this package cannot read as a unified diff.
	ErrPatchMalformed = errors.New("evo: Patch: malformed unified diff")
	// ErrPatchDoesNotApply is a well-formed edit whose context or removed lines
	// do not match the source it is applied to.
	ErrPatchDoesNotApply = errors.New("evo: Patch does not apply")
	// ErrPatchUnsupported is any patch form that cannot reduce to a desired
	// regular-file state. The specific forms below wrap it.
	ErrPatchUnsupported = errors.New("evo: Patch form unsupported")
	// ErrPatchDeleteUnsupported is a patch that deletes a file.
	ErrPatchDeleteUnsupported = fmt.Errorf("%w: file deletion", ErrPatchUnsupported)
	// ErrPatchRenameUnsupported is a patch that renames or copies a file.
	ErrPatchRenameUnsupported = fmt.Errorf("%w: rename or copy", ErrPatchUnsupported)
	// ErrPatchBinaryUnsupported is a binary patch.
	ErrPatchBinaryUnsupported = fmt.Errorf("%w: binary patch", ErrPatchUnsupported)
	// ErrPatchUnsafePath is a diff path that is absolute, climbs out of the
	// patch root, names a .git directory, or holds a NUL byte. Only
	// ParseStandard reports it; Parse keeps the older ErrMalformed.
	ErrPatchUnsafePath = errors.New("evo: Patch path is unsafe")
)

// FileEdit is one file's edit: the workspace-relative path it targets, whether
// it creates that file, the permission bits it sets (0 leaves the mode
// alone), and the hunks that turn the prior bytes into the desired ones.
//
// ParseStandard also yields deletions (Delete: the hunks must consume the
// whole file) and renames (From names the source, whose bytes the hunks, if
// any, turn into Path's).
type FileEdit struct {
	Path   string
	Create bool
	Delete bool
	From   string
	Mode   fs.FileMode
	Hunks  []PatchHunk
}

// ParsePatch reads diff into one File per changed path, in diff order. Text
// before the first file header (a commit message, a mail header) is
// ignored, as git apply ignores it.
func ParsePatch(diff []byte) ([]FileEdit, error) {
	return parse(diff, diffHeader.file)
}

// ParseStandardPatch is Parse with every standard unified-diff form
// representable: modify, create, delete, rename (with or without an edit),
// and mode change. Binary, copy, symlink, and submodule forms stay
// unsupported, and an unsafe path is ErrUnsafePath.
func ParseStandardPatch(diff []byte) ([]FileEdit, error) {
	return parse(diff, diffHeader.standardFile)
}

func parse(diff []byte, judge func(diffHeader) (FileEdit, error)) ([]FileEdit, error) {
	p := &diffParser{lines: splitLines(diff)}
	var files []FileEdit
	seen := map[string]bool{}
	for p.more() {
		header, ok, err := p.section()
		if err != nil {
			return nil, err
		}
		if !ok {
			p.next()
			continue
		}
		f, err := judge(header)
		if err != nil {
			return nil, err
		}
		for _, touched := range f.touched() {
			if seen[touched] {
				return nil, fmt.Errorf("%w: %s appears more than once", ErrPatchMalformed, touched)
			}
			seen[touched] = true
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%w: no file changes", ErrPatchMalformed)
	}
	return files, nil
}

// workspacePath validates a diff path and returns it cleaned in the host's
// separator form. A patch only edits paths beneath the root it is applied
// at: an absolute path or one that climbs out with ".." is rejected.
func workspacePath(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("%w: empty path", ErrPatchMalformed)
	}
	if path.IsAbs(raw) || filepath.IsAbs(raw) {
		return "", fmt.Errorf("%w: absolute path %q", ErrPatchMalformed, raw)
	}
	clean := path.Clean(raw)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: path %q escapes the patch root", ErrPatchMalformed, raw)
	}
	return filepath.FromSlash(clean), nil
}

// splitLines splits diff on "\n", dropping the final empty element a
// trailing newline leaves. "\r" stays: it is content in a CRLF file.
func splitLines(diff []byte) []string {
	if len(diff) == 0 {
		return nil
	}
	lines := strings.Split(string(diff), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// diffParser walks a diff's lines.
type diffParser struct {
	lines []string
	pos   int
}

func (p *diffParser) more() bool { return p.pos < len(p.lines) }

func (p *diffParser) peek() string { return p.lines[p.pos] }

// peekAt returns the line offset lines ahead, or "" past the end.
func (p *diffParser) peekAt(offset int) string {
	if p.pos+offset >= len(p.lines) {
		return ""
	}
	return p.lines[p.pos+offset]
}

func (p *diffParser) next() string {
	line := p.lines[p.pos]
	p.pos++
	return line
}
