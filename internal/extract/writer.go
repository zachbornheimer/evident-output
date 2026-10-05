package extract

import (
	"archive/tar"
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	dirMode        fs.FileMode = 0o755
	execFileMode   fs.FileMode = 0o755
	plainFileMode  fs.FileMode = 0o644
	ownerExecBit   fs.FileMode = 0o100
	maxLinkTarget              = 4096
	pathSeparator              = "/"
	parentSegment              = ".."
	currentSegment             = "."
)

// writer materialises validated entries under one staging root. All path
// handling goes through resolve, which never follows a symlink.
type writer struct {
	root    string   // absolute staging root
	strip   []string // Root segments, possibly empty
	entries int
}

func newWriter(dest, stripRoot string) (*writer, error) {
	abs, err := filepath.Abs(dest)
	if err != nil {
		return nil, err
	}
	return &writer{root: abs, strip: segments(stripRoot)}, nil
}

func segments(p string) []string {
	var out []string
	for s := range strings.SplitSeq(p, pathSeparator) {
		if s != "" && s != currentSegment {
			out = append(out, s)
		}
	}
	return out
}

// relPath validates an archive entry name and returns its segments below
// the staging root. skip is true for the Root directory itself.
func (w *writer) relPath(name string) (segs []string, skip bool, err error) {
	if name == "" || strings.ContainsAny(name, "\\\x00") {
		return nil, false, unsafe(name, "empty name or forbidden character")
	}
	if strings.HasPrefix(name, pathSeparator) {
		return nil, false, unsafe(name, "absolute path")
	}
	segs = segments(name)
	if slicesContain(segs, parentSegment) {
		return nil, false, unsafe(name, "parent traversal")
	}
	if len(w.strip) > 0 {
		if len(segs) < len(w.strip) || !equalSegments(segs[:len(w.strip)], w.strip) {
			return nil, false, unsafe(name, "outside Root")
		}
		segs = segs[len(w.strip):]
	}
	return segs, len(segs) == 0, nil
}

func slicesContain(s []string, v string) bool {
	return slices.Contains(s, v)
}

func equalSegments(a, b []string) bool {
	for i := range b {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// parentDir ensures every ancestor of segs is a real directory (never a
// symlink) and returns the entry's absolute path.
func (w *writer) parentDir(name string, segs []string) (string, error) {
	cur := w.root
	for _, seg := range segs[:len(segs)-1] {
		cur = filepath.Join(cur, seg)
		info, err := os.Lstat(cur)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			if err := os.Mkdir(cur, dirMode); err != nil {
				return "", err
			}
		case err != nil:
			return "", err
		case !info.IsDir():
			return "", unsafe(name, "path passes through a link or file")
		}
	}
	return filepath.Join(cur, segs[len(segs)-1]), nil
}

func (w *writer) addTar(ctx context.Context, h *tar.Header, body io.Reader) error {
	if h.Typeflag == tar.TypeXGlobalHeader {
		return nil
	}
	w.entries++
	segs, skip, err := w.relPath(h.Name)
	if err != nil {
		return err
	}
	switch h.Typeflag {
	case tar.TypeDir:
		if skip {
			return nil
		}
		return w.mkdir(h.Name, segs)
	case tar.TypeReg:
		if skip {
			return unsafe(h.Name, "Root is not a directory")
		}
		return w.file(ctx, h.Name, segs, fs.FileMode(h.Mode), body)
	case tar.TypeSymlink:
		if skip {
			return unsafe(h.Name, "Root is not a directory")
		}
		return w.symlink(h.Name, segs, h.Linkname)
	case tar.TypeLink:
		if skip {
			return unsafe(h.Name, "Root is not a directory")
		}
		return w.hardlink(h.Name, segs, h.Linkname)
	}
	return unsafe(h.Name, fmt.Sprintf("unsupported entry type %q", h.Typeflag))
}

func (w *writer) addZip(ctx context.Context, e *zip.File) error {
	w.entries++
	mode := e.Mode()
	isDir := mode.IsDir() || strings.HasSuffix(e.Name, pathSeparator)
	segs, skip, err := w.relPath(e.Name)
	if err != nil {
		return err
	}
	if skip && !isDir {
		return unsafe(e.Name, "Root is not a directory")
	}
	if skip {
		return nil
	}
	rc, err := e.Open()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	defer func() { _ = rc.Close() }() // read-only: nothing to flush
	switch {
	case isDir:
		return w.mkdir(e.Name, segs)
	case mode&fs.ModeSymlink != 0:
		target, err := io.ReadAll(io.LimitReader(&sourceReader{r: rc}, maxLinkTarget+1))
		if err != nil {
			return classifyCopy(ctx, err)
		}
		return w.symlink(e.Name, segs, string(target))
	case mode.IsRegular():
		return w.file(ctx, e.Name, segs, mode, &sourceReader{r: rc})
	}
	return unsafe(e.Name, "unsupported entry type")
}

func (w *writer) mkdir(name string, segs []string) error {
	path, err := w.parentDir(name, segs)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return os.Mkdir(path, dirMode)
	case err != nil:
		return err
	case !info.IsDir():
		return unsafe(name, "directory replaces a file or link")
	}
	return nil
}

func (w *writer) file(ctx context.Context, name string, segs []string, mode fs.FileMode, body io.Reader) error {
	path, err := w.parentDir(name, segs)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return unsafe(name, "duplicate or colliding entry")
	}
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, ctxReader{ctx: ctx, r: body})
	if copyErr != nil {
		_ = f.Close()
		return classifyCopy(ctx, copyErr)
	}
	perm := plainFileMode
	if mode&ownerExecBit != 0 {
		perm = execFileMode
	}
	if err := f.Chmod(perm); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// symlink creates a link whose target stays inside the tree. The target
// may begin with a run of ".." no longer than the link's own depth and
// must never use ".." again, so it cannot climb out through another link.
func (w *writer) symlink(name string, segs []string, target string) error {
	if len(target) > maxLinkTarget || target == "" || strings.ContainsAny(target, "\\\x00") || strings.HasPrefix(target, pathSeparator) {
		return unsafe(name, "link target not a contained relative path")
	}
	ups, descending := 0, false
	for s := range strings.SplitSeq(target, pathSeparator) {
		switch s {
		case "", currentSegment:
		case parentSegment:
			if descending {
				return unsafe(name, "link target climbs after descending")
			}
			ups++
		default:
			descending = true
		}
	}
	if ups > len(segs)-1 {
		return unsafe(name, "link target leaves the tree")
	}
	path, err := w.parentDir(name, segs)
	if err != nil {
		return err
	}
	if err := os.Symlink(target, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return unsafe(name, "duplicate or colliding entry")
		}
		return err
	}
	return nil
}

// hardlink links to an already extracted regular file inside the tree.
func (w *writer) hardlink(name string, segs []string, target string) error {
	tsegs, skip, err := w.relPath(target)
	if err != nil || skip {
		return unsafe(name, "hardlink target not inside the tree")
	}
	src := w.root
	for _, s := range tsegs {
		src = filepath.Join(src, s)
		info, err := os.Lstat(src)
		if err != nil || info.Mode()&fs.ModeSymlink != 0 {
			return unsafe(name, "hardlink target missing or through a link")
		}
	}
	if info, err := os.Lstat(src); err != nil || !info.Mode().IsRegular() {
		return unsafe(name, "hardlink target is not a regular file")
	}
	path, err := w.parentDir(name, segs)
	if err != nil {
		return err
	}
	if err := os.Link(src, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return unsafe(name, "duplicate or colliding entry")
		}
		return err
	}
	return nil
}
