package extract_test

// Adversarial hardening tests for evo.Extract. Each test is named for the
// weakness it guards; sources are in docs/zys-1382/adversarial-research.md.

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// advEntry is one archive member. Typeflag zero means a regular file; mode
// zero means 0644 (0755 for directories).
type advEntry struct {
	name, body, link string
	typeflag         byte
	mode             int64
}

func advRun(tb testing.TB, cfg evo.Config, fn func(context.Context) error) error {
	tb.Helper()
	cfg.Isolated, cfg.Plain = true, true
	cfg.Stdout, cfg.Stderr = io.Discard, io.Discard
	if cfg.StateDir == "" {
		cfg.StateDir = tb.TempDir()
	}
	out := evo.Init(cfg)
	defer func() { _ = out.Close() }()
	err := out.Task("adversarial").Define(fn).Wait()
	_ = out.Finish()
	return err
}

func advTarGz(tb testing.TB, path string, entries ...advEntry) evo.File {
	tb.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Linkname: e.link, Typeflag: e.typeflag, Mode: e.mode, ModTime: time.Unix(0, 0)}
		if hdr.Typeflag == 0 {
			hdr.Typeflag = tar.TypeReg
		}
		if hdr.Mode == 0 {
			hdr.Mode = 0o644
			if hdr.Typeflag == tar.TypeDir {
				hdr.Mode = 0o755
			}
		}
		if hdr.Typeflag == tar.TypeReg {
			hdr.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			tb.Fatal(err)
		}
		if hdr.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				tb.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		tb.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		tb.Fatal(err)
	}
	return evo.File{Path: path}
}

// advZip writes regular-file entries (names used verbatim) as a zip.
func advZip(tb testing.TB, path string, entries ...advEntry) evo.File {
	tb.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: e.name, Method: zip.Deflate})
		if err != nil {
			tb.Fatal(err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			tb.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		tb.Fatal(err)
	}
	return evo.File{Path: path}
}

func advFlat(prefix string, n int) []advEntry {
	entries := make([]advEntry, n)
	for i := range entries {
		entries[i] = advEntry{name: fmt.Sprintf("%s/f%04d.js", prefix, i), body: fmt.Sprint(i)}
	}
	return entries
}

// advExtract extracts archive into dest with root stripped.
func advExtract(tb testing.TB, dest string, archive evo.File, root string) error {
	tb.Helper()
	return advRun(tb, evo.Config{}, evo.Tree{Path: dest, Content: evo.Extract{File: archive, Root: root}}.Write)
}

// advSandbox is a work dir holding the archive and destination, plus a
// separate outside dir an escape would land in.
type advSandbox struct{ work, dest, outside string }

func newAdvSandbox(t *testing.T) advSandbox {
	t.Helper()
	work := t.TempDir()
	return advSandbox{work: work, dest: filepath.Join(work, "out", "pkg"), outside: t.TempDir()}
}

func (s advSandbox) archive(t *testing.T, entries ...advEntry) evo.File {
	t.Helper()
	return advTarGz(t, filepath.Join(s.work, "a.tgz"), entries...)
}

// requireRejected asserts the extraction failed with ErrExtractUnsafeEntry, nothing escaped, and no
// destination was published.
func (s advSandbox) requireRejected(t *testing.T, err error, escapes ...string) {
	t.Helper()
	if !errors.Is(err, evo.ErrExtractUnsafeEntry) {
		t.Fatalf("unsafe archive = %v, want ErrExtractUnsafeEntry", err)
	}
	s.requireNoEscape(t, escapes...)
	if _, statErr := os.Lstat(s.dest); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("a rejected archive still published %s", s.dest)
	}
}

func (s advSandbox) requireNoEscape(t *testing.T, escapes ...string) {
	t.Helper()
	for _, p := range escapes {
		if _, err := os.Lstat(p); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("archive escaped the destination: %s exists", p)
		}
	}
	if entries, _ := os.ReadDir(s.outside); len(entries) != 0 {
		t.Fatalf("archive wrote %q outside the destination", entries[0].Name())
	}
}

func TestAdversarial_ParentTraversalRejected(t *testing.T) {
	for _, format := range []string{"tar.gz", "zip"} {
		t.Run(format, func(t *testing.T) {
			s := newAdvSandbox(t)
			entries := []advEntry{{name: "ok.txt", body: "ok"}, {name: "../escape.txt", body: "x"}, {name: "a/../../../escape2.txt", body: "x"}}
			archive := s.archive(t, entries...)
			if format == "zip" {
				archive = advZip(t, filepath.Join(s.work, "a.zip"), entries...)
			}
			err := advExtract(t, s.dest, archive, "")
			s.requireRejected(t, err,
				filepath.Join(filepath.Dir(s.dest), "escape.txt"),
				filepath.Join(s.work, "escape2.txt"))
		})
	}
}

func TestAdversarial_AbsolutePathRejected(t *testing.T) {
	s := newAdvSandbox(t)
	target := filepath.Join(s.outside, "abs.txt")
	err := advExtract(t, s.dest, s.archive(t, advEntry{name: target, body: "x"}), "")
	s.requireRejected(t, err, target)
}

// node-tar CVE-2021-32803: a symlink to outside, then a file through it.
func TestAdversarial_SymlinkEscapeThenWriteThrough(t *testing.T) {
	s := newAdvSandbox(t)
	err := advExtract(t, s.dest, s.archive(t,
		advEntry{name: "link", typeflag: tar.TypeSymlink, link: s.outside},
		advEntry{name: "link/pwn", body: "x"},
	), "")
	s.requireRejected(t, err, filepath.Join(s.outside, "pwn"))
}

// PEP 706 data filter: links resolving outside the destination are refused
// even when nothing writes through them.
func TestAdversarial_SymlinkPointingOutsideRejected(t *testing.T) {
	for _, link := range []string{"../../../../../../etc/passwd", "/etc/passwd", "a/../../.."} {
		s := newAdvSandbox(t)
		err := advExtract(t, s.dest, s.archive(t, advEntry{name: "l", typeflag: tar.TypeSymlink, link: link}), "")
		s.requireRejected(t, err)
	}
}

// node-tar CVE-2021-32803/37701: a directory, then a same-name symlink, then
// a file through the (cached as directory) path.
func TestAdversarial_DirectoryReplacedBySymlink(t *testing.T) {
	s := newAdvSandbox(t)
	err := advExtract(t, s.dest, s.archive(t,
		advEntry{name: "d/", typeflag: tar.TypeDir},
		advEntry{name: "d", typeflag: tar.TypeSymlink, link: s.outside},
		advEntry{name: "d/pwn", body: "x"},
	), "")
	s.requireRejected(t, err, filepath.Join(s.outside, "pwn"))
}

// node-tar CVE-2021-37701: backslash is a filename byte on POSIX, not a
// separator; the safety check and the filesystem must agree.
func TestAdversarial_BackslashSeparatorConfusion(t *testing.T) {
	s := newAdvSandbox(t)
	err := advExtract(t, s.dest, s.archive(t,
		advEntry{name: `x\y`, typeflag: tar.TypeSymlink, link: s.outside},
		advEntry{name: `..\..\escape`, body: "x"},
		advEntry{name: "x/y/pwn", body: "x"},
	), "")
	s.requireRejected(t, err, filepath.Join(s.outside, "pwn"), filepath.Join(s.work, "escape"))
}

// Git CVE-2021-21300 analog: on a case-insensitive filesystem "Foo" (a
// symlink) and "foo/" alias.
func TestAdversarial_CaseInsensitiveSymlinkCollision(t *testing.T) {
	s := newAdvSandbox(t)
	err := advExtract(t, s.dest, s.archive(t,
		advEntry{name: "Foo", typeflag: tar.TypeSymlink, link: s.outside},
		advEntry{name: "foo/pwn", body: "x"},
	), "")
	s.requireRejected(t, err, filepath.Join(s.outside, "pwn"))
}

func TestAdversarial_HardlinkEscapeRejected(t *testing.T) {
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, link := range []string{victim, "../../../../../../../../" + victim} {
		s := newAdvSandbox(t)
		err := advExtract(t, s.dest, s.archive(t,
			advEntry{name: "h", typeflag: tar.TypeLink, link: link},
			advEntry{name: "h", body: "overwritten"},
		), "")
		if !errors.Is(err, evo.ErrExtractUnsafeEntry) {
			t.Fatalf("hardlink to %q = %v, want ErrExtractUnsafeEntry", link, err)
		}
		info, statErr := os.Stat(victim)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Nlink != 1 {
			t.Fatalf("extraction hardlinked an outside file (nlink %d)", st.Nlink)
		}
		if got, _ := os.ReadFile(victim); string(got) != "original" {
			t.Fatalf("outside file overwritten through a hardlink: %q", got)
		}
	}
}

func TestAdversarial_SpecialFilesRejected(t *testing.T) {
	for _, typeflag := range []byte{tar.TypeChar, tar.TypeBlock, tar.TypeFifo} {
		s := newAdvSandbox(t)
		err := advExtract(t, s.dest, s.archive(t, advEntry{name: "special", typeflag: typeflag}), "")
		s.requireRejected(t, err)
	}
}

func TestAdversarial_SetuidAndSetgidStripped(t *testing.T) {
	s := newAdvSandbox(t)
	err := advExtract(t, s.dest, s.archive(t, advEntry{name: "bin/tool", body: "#!/bin/sh\n", mode: 0o6755}), "")
	if errors.Is(err, evo.ErrExtractUnsafeEntry) {
		return // refusing the entry is also hardened
	}
	if err != nil {
		t.Fatalf("Write = %v, want success with the bits stripped or ErrExtractUnsafeEntry", err)
	}
	info, statErr := os.Stat(filepath.Join(s.dest, "bin", "tool"))
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Mode()&(fs.ModeSetuid|fs.ModeSetgid) != 0 {
		t.Fatalf("extracted mode %v keeps setuid/setgid", info.Mode())
	}
}

// Root stripping must work on path segments: "packagex/evil" is not under
// Root "package".
func TestAdversarial_RootPrefixIsPathSegment(t *testing.T) {
	s := newAdvSandbox(t)
	err := advExtract(t, s.dest, s.archive(t,
		advEntry{name: "package/ok.js", body: "ok"},
		advEntry{name: "packagex/evil.js", body: "x"},
	), "package")
	s.requireRejected(t, err)
	for _, p := range []string{filepath.Join(s.dest, "x", "evil.js"), filepath.Join(s.dest, "evil.js"), filepath.Join(s.dest, "x")} {
		if _, err := os.Lstat(p); err == nil {
			t.Fatalf("an entry outside Root was stripped by string prefix into %s", p)
		}
	}
}

func TestAdversarial_RootTraversalRejected(t *testing.T) {
	s := newAdvSandbox(t)
	err := advExtract(t, s.dest, s.archive(t,
		advEntry{name: "package/ok.js", body: "ok"},
		advEntry{name: "package/../../escape.js", body: "x"},
	), "package")
	s.requireRejected(t, err, filepath.Join(filepath.Dir(s.dest), "escape.js"), filepath.Join(s.work, "escape.js"))
}

// A malicious entry after hundreds of good ones publishes nothing.
func TestAdversarial_LateFailurePublishesNothing(t *testing.T) {
	s := newAdvSandbox(t)
	entries := append(advFlat("package", 300), advEntry{name: "package/../../late.js", body: "x"})
	err := advExtract(t, s.dest, s.archive(t, entries...), "package")
	s.requireRejected(t, err)
	if _, statErr := os.Lstat(filepath.Dir(s.dest)); statErr == nil {
		if entries, _ := os.ReadDir(filepath.Dir(s.dest)); len(entries) != 0 {
			t.Fatalf("late failure left %q beside the destination", entries[0].Name())
		}
	}
}

func TestAdversarial_CanceledExtractPublishesNothing(t *testing.T) {
	s := newAdvSandbox(t)
	archive := s.archive(t, advFlat("package", 2000)...)
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		return evo.Tree{Path: s.dest, Content: evo.Extract{File: archive, Root: "package"}}.Write(cctx)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Extract = %v, want context.Canceled", err)
	}
	if _, statErr := os.Lstat(s.dest); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatal("canceled Extract published the destination")
	}
}

// Structural: re-extracting the same archive over the current tree keeps
// every inode (no republish).
func TestAdversarial_CurrentTreeIsNotRewritten(t *testing.T) {
	s := newAdvSandbox(t)
	archive := s.archive(t, advFlat("package", 50)...)
	if err := advExtract(t, s.dest, archive, "package"); err != nil {
		t.Fatal(err)
	}
	before := advInodes(t, s.dest)
	if err := advExtract(t, s.dest, archive, "package"); err != nil {
		t.Fatal(err)
	}
	after := advInodes(t, s.dest)
	for rel, info := range before {
		if !os.SameFile(info, after[rel]) {
			t.Fatalf("%s was rewritten although the tree was current", rel)
		}
	}
}

func advInodes(t *testing.T, root string) map[string]fs.FileInfo {
	t.Helper()
	got := map[string]fs.FileInfo{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		got[path] = info
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func advZipSymlink(tb testing.TB, path, name, target string) evo.File {
	tb.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	hdr := &zip.FileHeader{Name: name, Method: zip.Store}
	hdr.SetMode(fs.ModeSymlink | 0o777)
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		tb.Fatal(err)
	}
	if _, err := w.Write([]byte(target)); err != nil {
		tb.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		tb.Fatal(err)
	}
	return evo.File{Path: path}
}

// Zip carries symlinks as mode bits; the same escape rules apply.
func TestAdversarial_ZipSymlinkEscapeRejected(t *testing.T) {
	s := newAdvSandbox(t)
	err := advExtract(t, s.dest, advZipSymlink(t, filepath.Join(s.work, "a.zip"), "link", s.outside), "")
	s.requireRejected(t, err)
}

func TestAdversarial_ZipAbsoluteAndBackslashNamesRejected(t *testing.T) {
	s := newAdvSandbox(t)
	target := filepath.Join(s.outside, "abs.txt")
	err := advExtract(t, s.dest, advZip(t, filepath.Join(s.work, "a.zip"),
		advEntry{name: target, body: "x"},
		advEntry{name: `..\..\escape.txt`, body: "x"},
	), "")
	s.requireRejected(t, err, target, filepath.Join(s.work, "escape.txt"))
}

// A tar that stores one name twice is ambiguous. The result is either a
// rejection that publishes nothing or exactly one whole version of the file.
func TestAdversarial_DuplicateEntryIsNotMerged(t *testing.T) {
	s := newAdvSandbox(t)
	err := advExtract(t, s.dest, s.archive(t,
		advEntry{name: "same.txt", body: "first"},
		advEntry{name: "same.txt", body: "second"},
	), "")
	if err != nil {
		if _, statErr := os.Lstat(s.dest); !errors.Is(statErr, fs.ErrNotExist) {
			t.Fatalf("a rejected duplicate-entry archive was published (%v)", err)
		}
		return
	}
	got, readErr := os.ReadFile(filepath.Join(s.dest, "same.txt"))
	if readErr != nil || (string(got) != "first" && string(got) != "second") {
		t.Fatalf("duplicate entry produced %q, %v; want exactly one whole version", got, readErr)
	}
}

// A symlink and a file with the same name must never leave the file's bytes
// behind the link's target.
func TestAdversarial_FileReplacingSymlinkDoesNotWriteThrough(t *testing.T) {
	s := newAdvSandbox(t)
	victim := filepath.Join(s.outside, "victim")
	err := advExtract(t, s.dest, s.archive(t,
		advEntry{name: "f", typeflag: tar.TypeSymlink, link: victim},
		advEntry{name: "f", body: "pwn"},
	), "")
	if _, statErr := os.Lstat(victim); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("a file entry was written through an earlier symlink entry (%v)", err)
	}
}

// Names differing only in case collide on case-insensitive filesystems
// (default macOS). Publishing may reject the archive, or keep both files
// intact; it must never silently drop one.
func TestAdversarial_CaseOnlyCollisionNeverDropsAFile(t *testing.T) {
	s := newAdvSandbox(t)
	err := advExtract(t, s.dest, s.archive(t,
		advEntry{name: "README.md", body: "upper"},
		advEntry{name: "readme.md", body: "lower"},
	), "")
	if err != nil {
		if _, statErr := os.Lstat(s.dest); !errors.Is(statErr, fs.ErrNotExist) {
			t.Fatalf("a rejected case-collision archive was published (%v)", err)
		}
		return
	}
	for name, want := range map[string]string{"README.md": "upper", "readme.md": "lower"} {
		if got, readErr := os.ReadFile(filepath.Join(s.dest, name)); readErr != nil || string(got) != want {
			t.Fatalf("%s = %q, %v; want %q (a collision silently dropped a file)", name, got, readErr, want)
		}
	}
}

// Decompression bomb: no size policy is specified, but whatever the policy,
// an expanding archive must stop at the deadline and publish nothing, and
// leave nothing beside the destination.
func TestAdversarial_DecompressionBombHonorsDeadlineAndLeavesNothing(t *testing.T) {
	const bombSize = 256 << 20
	s := newAdvSandbox(t)
	archivePath := filepath.Join(s.work, "bomb.tgz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "zeros.bin", Typeflag: tar.TypeReg, Mode: 0o644, Size: bombSize}); err != nil {
		t.Fatal(err)
	}
	chunk := make([]byte, 1<<20)
	for range bombSize / len(chunk) {
		if _, err := tw.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := errors.Join(tw.Close(), gz.Close(), f.Close()); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	runErr := advRun(t, evo.Config{}, func(ctx context.Context) error {
		cctx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
		defer cancel()
		return evo.Tree{Path: s.dest, Content: evo.Extract{File: evo.File{Path: archivePath}}}.Write(cctx)
	})
	if runErr == nil {
		t.Fatal("a 256 MiB expansion finished inside a 20ms deadline without error")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("extraction ignored the deadline for %v", elapsed)
	}
	if _, statErr := os.Lstat(s.dest); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatal("an interrupted extraction published the destination")
	}
	if entries, _ := os.ReadDir(filepath.Dir(s.dest)); len(entries) != 0 {
		t.Fatalf("an interrupted extraction left %q beside the destination", entries[0].Name())
	}
}

func BenchmarkExtract_5000FileTarGz(b *testing.B) {
	work := b.TempDir()
	archive := advTarGz(b, filepath.Join(work, "a.tgz"), advFlat("package", 5000)...)
	err := advRun(b, evo.Config{}, func(ctx context.Context) error {
		for i := 0; b.Loop(); i++ {
			dest := filepath.Join(work, fmt.Sprint(i))
			if err := (evo.Tree{Path: dest, Content: evo.Extract{File: archive, Root: "package"}}).Write(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
}

func BenchmarkExtract_2000FileZip(b *testing.B) {
	work := b.TempDir()
	archive := advZip(b, filepath.Join(work, "a.zip"), advFlat("package", 2000)...)
	err := advRun(b, evo.Config{}, func(ctx context.Context) error {
		for i := 0; b.Loop(); i++ {
			dest := filepath.Join(work, fmt.Sprint(i))
			if err := (evo.Tree{Path: dest, Content: evo.Extract{File: archive, Root: "package"}}).Write(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
}
