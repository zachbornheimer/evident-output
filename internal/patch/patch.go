// Package patch reads unified text diffs (plain `diff -u` and git's
// extended headers) into per-file edits and applies an edit to a file's
// prior bytes. It is pure: it never touches the filesystem. Forms a file's
// desired state cannot express faithfully — deletion, rename/copy, binary,
// and non-regular file types — are rejected, never approximated.
package patch

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
	// ErrMalformed is a diff this package cannot read as a unified diff.
	ErrMalformed = errors.New("evo: Patch: malformed unified diff")
	// ErrDoesNotApply is a well-formed edit whose context or removed lines
	// do not match the source it is applied to.
	ErrDoesNotApply = errors.New("evo: Patch does not apply")
	// ErrUnsupported is any patch form that cannot reduce to a desired
	// regular-file state. The specific forms below wrap it.
	ErrUnsupported = errors.New("evo: Patch form unsupported")
	// ErrDeleteUnsupported is a patch that deletes a file.
	ErrDeleteUnsupported = fmt.Errorf("%w: file deletion", ErrUnsupported)
	// ErrRenameUnsupported is a patch that renames or copies a file.
	ErrRenameUnsupported = fmt.Errorf("%w: rename or copy", ErrUnsupported)
	// ErrBinaryUnsupported is a binary patch.
	ErrBinaryUnsupported = fmt.Errorf("%w: binary patch", ErrUnsupported)
)

// File is one file's edit: the workspace-relative path it targets, whether
// it creates that file, the permission bits it sets (0 leaves the mode
// alone), and the hunks that turn the prior bytes into the desired ones.
type File struct {
	Path   string
	Create bool
	Mode   fs.FileMode
	Hunks  []Hunk
}

// Parse reads diff into one File per changed path, in diff order. Text
// before the first file header (a commit message, a mail header) is
// ignored, as git apply ignores it.
func Parse(diff []byte) ([]File, error) {
	p := &parser{lines: splitLines(diff)}
	var files []File
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
		f, err := header.file()
		if err != nil {
			return nil, err
		}
		if seen[f.Path] {
			return nil, fmt.Errorf("%w: %s appears more than once", ErrMalformed, f.Path)
		}
		seen[f.Path] = true
		files = append(files, f)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%w: no file changes", ErrMalformed)
	}
	return files, nil
}

// workspacePath validates a diff path and returns it cleaned in the host's
// separator form. A patch only edits paths beneath the root it is applied
// at: an absolute path or one that climbs out with ".." is rejected.
func workspacePath(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("%w: empty path", ErrMalformed)
	}
	if path.IsAbs(raw) || filepath.IsAbs(raw) {
		return "", fmt.Errorf("%w: absolute path %q", ErrMalformed, raw)
	}
	clean := path.Clean(raw)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: path %q escapes the patch root", ErrMalformed, raw)
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

// parser walks a diff's lines.
type parser struct {
	lines []string
	pos   int
}

func (p *parser) more() bool { return p.pos < len(p.lines) }

func (p *parser) peek() string { return p.lines[p.pos] }

// peekAt returns the line offset lines ahead, or "" past the end.
func (p *parser) peekAt(offset int) string {
	if p.pos+offset >= len(p.lines) {
		return ""
	}
	return p.lines[p.pos+offset]
}

func (p *parser) next() string {
	line := p.lines[p.pos]
	p.pos++
	return line
}
