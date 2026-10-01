// Package extract_test is the ZYS-1382 execution contract for evo.Extract,
// the Tree content producer that unpacks an archive File safely and
// publishes the whole tree at once. Names and open-syntax choices:
// docs/zys-1382/contract-decisions.md.
package extract_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

var _ evo.TreeContent = evo.Extract{}

// contractRun runs fn inside one Task's Define callback on an isolated
// Output and returns fn's own error.
func contractRun(t *testing.T, cfg evo.Config, fn func(ctx context.Context) error) error {
	t.Helper()
	cfg.Isolated, cfg.Plain = true, true
	cfg.Stdout, cfg.Stderr = io.Discard, io.Discard
	if cfg.StateDir == "" {
		cfg.StateDir = t.TempDir()
	}
	out := evo.Init(cfg)
	var (
		ran bool
		got error
	)
	task := out.Task("contract").Define(func(ctx context.Context) error {
		ran = true
		got = fn(ctx)
		return got
	})
	_ = task.Wait()
	_ = out.Finish()
	if !ran {
		t.Fatalf("contract Task callback never ran")
	}
	return got
}

// plant writes files (slash-separated relative path -> content) under root.
func plant(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// entry is one archive member; a Dir entry has empty Body and a trailing
// slash in its Name.
type entry struct {
	Name string
	Body string
	Mode fs.FileMode
	Dir  bool
}

func regular(name, body string) entry { return entry{Name: name, Body: body, Mode: 0o644} }

func tarBytes(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.Name, Mode: int64(e.Mode)}
		if e.Dir {
			hdr.Typeflag = tar.TypeDir
			if hdr.Mode == 0 {
				hdr.Mode = 0o755
			}
		} else {
			hdr.Typeflag = tar.TypeReg
			hdr.Size = int64(len(e.Body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if !e.Dir {
			if _, err := tw.Write([]byte(e.Body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func zipBytes(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.Name, Method: zip.Deflate}
		if e.Dir {
			hdr.SetMode(fs.ModeDir | 0o755)
		} else {
			hdr.SetMode(e.Mode)
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		if !e.Dir {
			if _, err := w.Write([]byte(e.Body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// archiveFile writes data under dir and returns it as a File.
func archiveFile(t *testing.T, dir, name string, data []byte) evo.File {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return evo.File{Path: path}
}

func onDisk(t *testing.T, root string) map[string]string {
	t.Helper()
	got := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		body, readErr := os.ReadFile(path)
		got[filepath.ToSlash(rel)] = string(body)
		return readErr
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func equalFiles(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

var packaged = []entry{
	{Name: "package/", Dir: true},
	regular("package/package.json", `{"name":"pkg"}`),
	{Name: "package/lib/", Dir: true},
	regular("package/lib/index.js", "module.exports = 1\n"),
	regular("package/lib/deep/util.js", "exports.util = true\n"),
}

var packagedFiles = map[string]string{
	"package.json":     `{"name":"pkg"}`,
	"lib/index.js":     "module.exports = 1\n",
	"lib/deep/util.js": "exports.util = true\n",
}

func TestExtractIsAPlainStructLiteral(t *testing.T) {
	x := evo.Extract{File: evo.File{Path: "pkg.tgz"}, Root: "package"}
	tree := evo.Tree{Path: "node_modules/pkg", Content: x}
	if tree.Content == nil || x.File.Path != "pkg.tgz" || x.Root != "package" {
		t.Fatalf("Extract literal did not round-trip: %+v", tree)
	}
}

func TestExtractSupportsTarGzTarAndZip(t *testing.T) {
	cases := []struct {
		name string
		file string
		data func(t *testing.T) []byte
	}{
		{"tar.gz", "pkg.tgz", func(t *testing.T) []byte { return gzipBytes(t, tarBytes(t, packaged)) }},
		{"tar", "pkg.tar", func(t *testing.T) []byte { return tarBytes(t, packaged) }},
		{"zip", "pkg.zip", func(t *testing.T) []byte { return zipBytes(t, packaged) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work := t.TempDir()
			dest := filepath.Join(work, "pkg")
			tree := evo.Tree{Path: dest, Content: evo.Extract{File: archiveFile(t, work, tc.file, tc.data(t)), Root: "package"}}
			if err := contractRun(t, evo.Config{}, tree.Write); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if got := onDisk(t, dest); !equalFiles(got, packagedFiles) {
				t.Fatalf("extracted = %v, want %v", got, packagedFiles)
			}
		})
	}
}

func TestExtractFormatIsDetectedFromContentNotExtension(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	// A gzip tarball named like a zip.
	archive := archiveFile(t, work, "mislabeled.zip", gzipBytes(t, tarBytes(t, packaged)))
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archive, Root: "package"}}
	if err := contractRun(t, evo.Config{}, tree.Write); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := onDisk(t, dest); !equalFiles(got, packagedFiles) {
		t.Fatalf("extracted = %v, want %v", got, packagedFiles)
	}
}

func TestExtractRootStripsOneLeadingComponent(t *testing.T) {
	cases := []struct {
		name string
		root string
		want map[string]string
	}{
		{"with Root the prefix is removed", "package", packagedFiles},
		{"without Root the prefix is kept", "", map[string]string{
			"package/package.json": `{"name":"pkg"}`, "package/lib/index.js": "module.exports = 1\n", "package/lib/deep/util.js": "exports.util = true\n",
		}},
		{"Root tolerates a trailing slash", "package/", packagedFiles},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work := t.TempDir()
			dest := filepath.Join(work, "pkg")
			tree := evo.Tree{Path: dest, Content: evo.Extract{File: archiveFile(t, work, "pkg.tgz", gzipBytes(t, tarBytes(t, packaged))), Root: tc.root}}
			if err := contractRun(t, evo.Config{}, tree.Write); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if got := onDisk(t, dest); !equalFiles(got, tc.want) {
				t.Fatalf("extracted = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExtractRootMustMatchEveryEntry(t *testing.T) {
	entries := append(slices.Clone(packaged), regular("outside/stray.txt", "stray"))
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archiveFile(t, work, "pkg.tgz", gzipBytes(t, tarBytes(t, entries))), Root: "package"}}
	if err := contractRun(t, evo.Config{}, tree.Write); !errors.Is(err, evo.ErrExtractUnsafeEntry) {
		t.Fatalf("Write = %v, want ErrExtractUnsafeEntry for an entry outside Root", err)
	}
	if _, err := os.Lstat(dest); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a rejected archive was partly published")
	}
}

func TestExtractPreservesExecutableBits(t *testing.T) {
	entries := []entry{
		{Name: "bin/tool", Body: "#!/bin/sh\n", Mode: 0o755},
		regular("README", "text"),
	}
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archiveFile(t, work, "pkg.tar", tarBytes(t, entries))}}
	if err := contractRun(t, evo.Config{}, tree.Write); err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := os.Stat(filepath.Join(dest, "bin", "tool"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("bin/tool mode = %v, want the owner execute bit kept", info.Mode().Perm())
	}
	if info, err = os.Stat(filepath.Join(dest, "README")); err != nil || info.Mode().Perm()&0o111 != 0 {
		t.Fatalf("README mode = %v, want no execute bits", info.Mode().Perm())
	}
}

func TestExtractRejectsUnsafeEntries(t *testing.T) {
	cases := []struct {
		name    string
		root    string
		entries []entry
		zip     bool
	}{
		{name: "path traversal", entries: []entry{regular("../escape.txt", "x")}},
		{name: "nested path traversal", entries: []entry{regular("a/../../escape.txt", "x")}},
		{name: "absolute path", entries: []entry{regular("/etc/passwd", "x")}},
		{name: "traversal under Root", root: "package", entries: []entry{regular("package/ok.txt", "ok"), regular("package/../../escape.txt", "x")}},
		{name: "entry outside Root", root: "package", entries: []entry{regular("package/ok.txt", "ok"), regular("elsewhere/x.txt", "x")}},
		{name: "zip path traversal", zip: true, entries: []entry{regular("../escape.txt", "x")}},
		{name: "zip absolute path", zip: true, entries: []entry{regular("/etc/passwd", "x")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work := t.TempDir()
			dest := filepath.Join(work, "out", "pkg")
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				t.Fatal(err)
			}
			data, name := tarBytes(t, tc.entries), "evil.tar"
			if tc.zip {
				data, name = zipBytes(t, tc.entries), "evil.zip"
			}
			tree := evo.Tree{Path: dest, Content: evo.Extract{File: archiveFile(t, work, name, data), Root: tc.root}}
			if err := contractRun(t, evo.Config{}, tree.Write); !errors.Is(err, evo.ErrExtractUnsafeEntry) {
				t.Fatalf("Write = %v, want ErrExtractUnsafeEntry", err)
			}
			for _, escaped := range []string{filepath.Join(work, "escape.txt"), filepath.Join(work, "out", "escape.txt")} {
				if _, err := os.Lstat(escaped); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("an escaping entry was written to %s", escaped)
				}
			}
			if _, err := os.Lstat(dest); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("a rejected archive was partly published")
			}
		})
	}
}

func TestExtractRejectsLinkEscapesAndSpecialFiles(t *testing.T) {
	link := func(name, target string, typ byte) entry {
		return entry{Name: name + "\x00" + target + "\x00" + string(typ)}
	}
	// tarLink builds a tar with one link/special entry encoded by link().
	tarLink := func(t *testing.T, entries []entry) []byte {
		t.Helper()
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		for _, e := range entries {
			parts := bytes.Split([]byte(e.Name), []byte{0})
			if len(parts) != 3 {
				hdr := &tar.Header{Name: e.Name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(e.Body))}
				if err := tw.WriteHeader(hdr); err != nil {
					t.Fatal(err)
				}
				if _, err := tw.Write([]byte(e.Body)); err != nil {
					t.Fatal(err)
				}
				continue
			}
			hdr := &tar.Header{Name: string(parts[0]), Linkname: string(parts[1]), Typeflag: parts[2][0], Mode: 0o644}
			if err := tw.WriteHeader(hdr); err != nil {
				t.Fatal(err)
			}
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	cases := []struct {
		name    string
		entries []entry
	}{
		{"symlink escaping the tree", []entry{link("escape", "../../outside", tar.TypeSymlink)}},
		{"absolute symlink", []entry{link("escape", "/etc", tar.TypeSymlink)}},
		{"symlink then write through it", []entry{link("dir", "..", tar.TypeSymlink), regular("dir/escape.txt", "x")}},
		{"hardlink escaping the tree", []entry{link("escape", "../../outside", tar.TypeLink)}},
		{"character device", []entry{link("dev", "", tar.TypeChar)}},
		{"block device", []entry{link("dev", "", tar.TypeBlock)}},
		{"fifo", []entry{link("pipe", "", tar.TypeFifo)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work := t.TempDir()
			dest := filepath.Join(work, "out", "pkg")
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				t.Fatal(err)
			}
			tree := evo.Tree{Path: dest, Content: evo.Extract{File: archiveFile(t, work, "evil.tar", tarLink(t, tc.entries))}}
			if err := contractRun(t, evo.Config{}, tree.Write); !errors.Is(err, evo.ErrExtractUnsafeEntry) {
				t.Fatalf("Write = %v, want ErrExtractUnsafeEntry", err)
			}
			if _, err := os.Lstat(dest); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("a rejected archive was partly published")
			}
			if _, err := os.Lstat(filepath.Join(work, "escape.txt")); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("an entry escaped through a link")
			}
		})
	}
}

func TestExtractPublishesTheWholeTreeAtomically(t *testing.T) {
	work := t.TempDir()
	parent := filepath.Join(work, "out")
	dest := filepath.Join(parent, "pkg")
	plant(t, dest, map[string]string{"old.txt": "stale"})
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archiveFile(t, work, "pkg.tgz", gzipBytes(t, tarBytes(t, packaged))), Root: "package"}}
	if err := contractRun(t, evo.Config{}, tree.Write); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := onDisk(t, dest); !equalFiles(got, packagedFiles) {
		t.Fatalf("extracted = %v, want exactly %v (old entries replaced)", got, packagedFiles)
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "pkg" {
		t.Fatalf("Write left staging beside the destination: %v", entries)
	}
}

func TestExtractMalformedArchiveNeverPublishes(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"random bytes", []byte("this is not an archive at all")},
		{"empty file", nil},
		{"truncated tar.gz", func() []byte { return []byte{0x1f, 0x8b, 0x08, 0x00, 0x00} }()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work := t.TempDir()
			dest := filepath.Join(work, "pkg")
			tree := evo.Tree{Path: dest, Content: evo.Extract{File: archiveFile(t, work, "bad.tgz", tc.data)}}
			if err := contractRun(t, evo.Config{}, tree.Write); !errors.Is(err, evo.ErrExtractMalformed) {
				t.Fatalf("Write = %v, want ErrExtractMalformed", err)
			}
			if _, err := os.Lstat(dest); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("a malformed archive was published")
			}
		})
	}
}

func TestExtractMissingArchiveIsNotExist(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: evo.File{Path: filepath.Join(work, "missing.tgz")}}}
	if err := contractRun(t, evo.Config{}, tree.Write); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Write = %v, want fs.ErrNotExist", err)
	}
}

func TestExtractVerifyComparesDiskToTheArchive(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archiveFile(t, work, "pkg.tgz", gzipBytes(t, tarBytes(t, packaged))), Root: "package"}}
	if err := contractRun(t, evo.Config{}, tree.Write); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := contractRun(t, evo.Config{}, tree.Verify); err != nil {
		t.Fatalf("Verify after Write = %v", err)
	}
	plant(t, dest, map[string]string{"lib/index.js": "tampered"})
	if err := contractRun(t, evo.Config{}, tree.Verify); !errors.Is(err, evo.ErrVerifyMismatch) {
		t.Fatalf("Verify of a tampered tree = %v, want ErrVerifyMismatch", err)
	}
}

func TestExtractHonorsContextCancellation(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archiveFile(t, work, "pkg.tgz", gzipBytes(t, tarBytes(t, packaged))), Root: "package"}}
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		return tree.Write(ctx)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Write = %v, want context.Canceled", err)
	}
	if _, statErr := os.Lstat(dest); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("a cancelled extraction published the destination")
	}
}

func TestExtractRejectionLeavesAnExistingTreeUntouched(t *testing.T) {
	work := t.TempDir()
	parent := filepath.Join(work, "out")
	dest := filepath.Join(parent, "pkg")
	plant(t, dest, map[string]string{"keep.txt": "previous"})
	evil := tarBytes(t, []entry{regular("fine.txt", "new"), regular("../escape.txt", "x")})
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archiveFile(t, work, "evil.tar", evil)}}
	if err := contractRun(t, evo.Config{}, tree.Write); !errors.Is(err, evo.ErrExtractUnsafeEntry) {
		t.Fatalf("Write = %v, want ErrExtractUnsafeEntry", err)
	}
	if got := onDisk(t, dest); !equalFiles(got, map[string]string{"keep.txt": "previous"}) {
		t.Fatalf("a rejected archive changed the previous tree: %v", got)
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 1 {
		t.Fatalf("a rejected archive left staging beside the destination: %v", entries)
	}
}

func TestExtractMalformedArchiveLeavesAnExistingTreeUntouched(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	plant(t, dest, map[string]string{"keep.txt": "previous"})
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archiveFile(t, work, "bad.tgz", []byte("not an archive"))}}
	if err := contractRun(t, evo.Config{}, tree.Write); !errors.Is(err, evo.ErrExtractMalformed) {
		t.Fatalf("Write = %v, want ErrExtractMalformed", err)
	}
	if got := onDisk(t, dest); !equalFiles(got, map[string]string{"keep.txt": "previous"}) {
		t.Fatalf("a malformed archive changed the previous tree: %v", got)
	}
}

func TestExtractMalformedVariants(t *testing.T) {
	valid := gzipBytes(t, tarBytes(t, packaged))
	validZip := zipBytes(t, packaged)
	cases := []struct {
		name string
		data []byte
	}{
		{"gzip of garbage", gzipBytes(t, []byte("garbage that is not a tar stream, padded"+string(make([]byte, 600))))},
		{"tar with zero entries", tarBytes(t, nil)},
		{"gzip with zero entries", gzipBytes(t, tarBytes(t, nil))},
		{"truncated gzip body", valid[:len(valid)/2]},
		{"truncated zip", validZip[:len(validZip)/2]},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work := t.TempDir()
			dest := filepath.Join(work, "pkg")
			tree := evo.Tree{Path: dest, Content: evo.Extract{File: archiveFile(t, work, "bad.bin", tc.data)}}
			if err := contractRun(t, evo.Config{}, tree.Write); !errors.Is(err, evo.ErrExtractMalformed) {
				t.Fatalf("Write = %v, want ErrExtractMalformed", err)
			}
			if _, err := os.Lstat(dest); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("a malformed archive was published")
			}
		})
	}
}

func TestExtractZipPreservesExecutableBits(t *testing.T) {
	entries := []entry{{Name: "bin/tool", Body: "#!/bin/sh\n", Mode: 0o755}, regular("README", "text")}
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archiveFile(t, work, "pkg.zip", zipBytes(t, entries))}}
	if err := contractRun(t, evo.Config{}, tree.Write); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if info, err := os.Stat(filepath.Join(dest, "bin", "tool")); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("bin/tool = %v, %v; want the owner execute bit kept", info, err)
	}
	if info, err := os.Stat(filepath.Join(dest, "README")); err != nil || info.Mode().Perm()&0o111 != 0 {
		t.Fatalf("README = %v, %v; want no execute bits", info, err)
	}
}

func TestExtractIgnoresTheArchiveFilesContentAndMode(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	archive := archiveFile(t, work, "pkg.tgz", gzipBytes(t, tarBytes(t, packaged)))
	archive.Content, archive.Mode = evo.Bytes("irrelevant"), 0o600
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archive, Root: "package"}}
	if err := contractRun(t, evo.Config{}, tree.Write); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := onDisk(t, dest); !equalFiles(got, packagedFiles) {
		t.Fatalf("extracted = %v, want %v (the archive File is read from Path)", got, packagedFiles)
	}
	if got, err := os.ReadFile(archive.Path); err != nil || len(got) < 10 || string(got) == "irrelevant" {
		t.Fatalf("Write touched the archive file: %q, %v", got, err)
	}
}

func TestExtractKeepsLinksThatStayInsideTheTree(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	add := func(h *tar.Header, body string) {
		t.Helper()
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	add(&tar.Header{Name: "lib/real.js", Typeflag: tar.TypeReg, Mode: 0o644, Size: 4}, "real")
	add(&tar.Header{Name: "lib/soft.js", Typeflag: tar.TypeSymlink, Linkname: "real.js", Mode: 0o777}, "")
	add(&tar.Header{Name: "lib/hard.js", Typeflag: tar.TypeLink, Linkname: "lib/real.js", Mode: 0o644}, "")
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archiveFile(t, work, "links.tar", buf.Bytes())}}
	if err := contractRun(t, evo.Config{}, tree.Write); err != nil {
		t.Fatalf("Write rejected links that stay inside the tree: %v", err)
	}
	if target, err := os.Readlink(filepath.Join(dest, "lib", "soft.js")); err != nil || target != "real.js" {
		t.Fatalf("soft.js = %q, %v; want a symlink to real.js", target, err)
	}
	for _, name := range []string{"soft.js", "hard.js"} {
		if got, err := os.ReadFile(filepath.Join(dest, "lib", name)); err != nil || string(got) != "real" {
			t.Fatalf("%s reads %q, %v; want real", name, got, err)
		}
	}
}

func TestExtractVerifyOfAMissingTreeIsAMismatch(t *testing.T) {
	work := t.TempDir()
	tree := evo.Tree{Path: filepath.Join(work, "pkg"), Content: evo.Extract{File: archiveFile(t, work, "pkg.tgz", gzipBytes(t, tarBytes(t, packaged))), Root: "package"}}
	if err := contractRun(t, evo.Config{}, tree.Verify); !errors.Is(err, evo.ErrVerifyMismatch) {
		t.Fatalf("Verify of a missing tree = %v, want ErrVerifyMismatch", err)
	}
	if _, err := os.Lstat(tree.Path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Verify created the destination")
	}
}

func TestExtractVerifyDetectsAddedAndRemovedFiles(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archiveFile(t, work, "pkg.tgz", gzipBytes(t, tarBytes(t, packaged))), Root: "package"}}
	if err := contractRun(t, evo.Config{}, tree.Write); err != nil {
		t.Fatalf("Write: %v", err)
	}
	plant(t, dest, map[string]string{"extra.txt": "added"})
	if err := contractRun(t, evo.Config{}, tree.Verify); !errors.Is(err, evo.ErrVerifyMismatch) {
		t.Fatalf("Verify with an added file = %v, want ErrVerifyMismatch", err)
	}
	if err := os.Remove(filepath.Join(dest, "extra.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dest, "package.json")); err != nil {
		t.Fatal(err)
	}
	if err := contractRun(t, evo.Config{}, tree.Verify); !errors.Is(err, evo.ErrVerifyMismatch) {
		t.Fatalf("Verify with a removed file = %v, want ErrVerifyMismatch", err)
	}
}

func TestExtractedTreeEqualsAnIdenticalHandBuiltTree(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	built := filepath.Join(work, "built")
	plant(t, built, packagedFiles)
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archiveFile(t, work, "pkg.tgz", gzipBytes(t, tarBytes(t, packaged))), Root: "package"}}
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		if err := tree.Write(ctx); err != nil {
			return err
		}
		same, err := tree.Equal(ctx, evo.Tree{Path: built})
		if err != nil {
			return err
		}
		if !same {
			t.Errorf("an extracted tree is not Equal to a hand-built tree with the same entries")
		}
		files, err := tree.Read(ctx)
		if err != nil {
			return err
		}
		var rels []string
		for _, f := range files {
			rel, _ := filepath.Rel(dest, f.Path)
			rels = append(rels, filepath.ToSlash(rel))
			if f.Content != nil {
				t.Errorf("Tree.Read returned content for %s; want content-lazy", rel)
			}
		}
		if want := []string{"lib/deep/util.js", "lib/index.js", "package.json"}; !slices.Equal(rels, want) {
			t.Errorf("Tree.Read = %v, want %v", rels, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestExtractWriteInDryRunMutatesNothing(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archiveFile(t, work, "pkg.tgz", gzipBytes(t, tarBytes(t, packaged))), Root: "package"}}
	if err := contractRun(t, evo.Config{DryRun: true}, tree.Write); err != nil {
		t.Fatalf("DryRun Write = %v, want nil", err)
	}
	if entries, _ := os.ReadDir(work); len(entries) != 1 {
		t.Fatalf("DryRun Write created %v beside the archive", entries)
	}
}

func TestExtractWriteOutsideATaskIsRefused(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	tree := evo.Tree{Path: dest, Content: evo.Extract{File: archiveFile(t, work, "pkg.tgz", gzipBytes(t, tarBytes(t, packaged))), Root: "package"}}
	if err := tree.Write(context.Background()); !errors.Is(err, evo.ErrNoTaskContext) {
		t.Fatalf("Write outside a Task = %v, want ErrNoTaskContext", err)
	}
	if _, err := os.Lstat(dest); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Write outside a Task mutated the destination")
	}
}

func TestExtractWithAnEmptyTreePathIsRefused(t *testing.T) {
	work := t.TempDir()
	tree := evo.Tree{Content: evo.Extract{File: archiveFile(t, work, "pkg.tgz", gzipBytes(t, tarBytes(t, packaged)))}}
	if err := contractRun(t, evo.Config{}, tree.Write); !errors.Is(err, evo.ErrPathMissing) {
		t.Fatalf("Write with empty Path = %v, want ErrPathMissing", err)
	}
}

func TestExtractWithAnEmptyArchivePathIsRefused(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "pkg")
	tree := evo.Tree{Path: dest, Content: evo.Extract{}}
	if err := contractRun(t, evo.Config{}, tree.Write); !errors.Is(err, evo.ErrPathMissing) {
		t.Fatalf("Write with an empty archive Path = %v, want ErrPathMissing", err)
	}
	if _, err := os.Lstat(dest); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Write with an empty archive Path published the destination")
	}
}
