// Package file_test is the ZYS-1382 execution contract for evo.File: one
// regular file reconciled through Read, Write, Verify, Equal, Remove, and
// Checksum. Names and open-syntax choices: docs/zys-1382/contract-decisions.md.
package file_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// Sealed content producers a File accepts.
var (
	_ evo.FileContent = evo.Bytes([]byte(nil))
	_ evo.FileContent = evo.Download{}
)

// Method expressions only compile when every method has a value receiver,
// and pin the exact shared vocabulary signatures.
var (
	_ func(evo.File, context.Context) ([]byte, error)         = evo.File.Read
	_ func(evo.File, context.Context) error                   = evo.File.Write
	_ func(evo.File, context.Context) error                   = evo.File.Verify
	_ func(evo.File, context.Context, evo.File) (bool, error) = evo.File.Equal
	_ func(evo.File, context.Context) error                   = evo.File.Remove
	_ func(evo.File, context.Context) (string, error)         = evo.File.Checksum
)

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

func mustWrite(t *testing.T, path string, data []byte, mode fs.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func TestFileIsAPlainStructLiteral(t *testing.T) {
	data := []byte("hello")
	f := evo.File{Path: "a.txt", Content: evo.Bytes(data), Mode: 0o644}
	if f.Path != "a.txt" || f.Mode != fs.FileMode(0o644) || f.Content == nil {
		t.Fatalf("File literal fields did not round-trip: %+v", f)
	}
	d := evo.File{Path: "b.tgz", Content: evo.Download{URL: "https://example.invalid/b.tgz", Integrity: "sha512-AAAA"}}
	if d.Content == nil {
		t.Fatalf("File literal with Download content lost its Content")
	}
}

func TestFileWriteEstablishesDesiredContent(t *testing.T) {
	cases := []struct {
		name    string
		before  []byte // nil: path is missing
		desired []byte
	}{
		{"creates a missing file", nil, []byte("fresh\n")},
		{"replaces differing content", []byte("old\n"), []byte("new\n")},
		{"keeps matching content", []byte("same\n"), []byte("same\n")},
		{"empty Bytes establishes an empty file", []byte("old\n"), []byte{}},
		{"nil Bytes establishes an empty file", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "f.txt")
			if tc.before != nil {
				mustWrite(t, path, tc.before, 0o644)
			}
			err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
				return evo.File{Path: path, Content: evo.Bytes(tc.desired)}.Write(ctx)
			})
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			if string(got) != string(tc.desired) {
				t.Fatalf("content = %q, want %q", got, tc.desired)
			}
		})
	}
}

func TestFileWriteAppliesMode(t *testing.T) {
	cases := []struct {
		name   string
		before fs.FileMode // 0: path is missing
		mode   fs.FileMode
		want   fs.FileMode
	}{
		{"zero Mode creates 0644", 0, 0, 0o644},
		{"explicit 0600 on create", 0, 0o600, 0o600},
		{"explicit 0755 on create", 0, 0o755, 0o755},
		{"explicit Mode changes an existing file", 0o644, 0o755, 0o755},
		{"zero Mode preserves an existing file's mode", 0o600, 0, 0o600},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "f.txt")
			if tc.before != 0 {
				mustWrite(t, path, []byte("old"), tc.before)
			}
			err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
				return evo.File{Path: path, Content: evo.Bytes("new"), Mode: tc.mode}.Write(ctx)
			})
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != tc.want {
				t.Fatalf("mode = %v, want %v", info.Mode().Perm(), tc.want)
			}
		})
	}
}

func TestFileWriteAlreadySatisfiedDoesNotRepublish(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	mustWrite(t, path, []byte("same"), 0o644)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	err = contractRun(t, evo.Config{}, func(ctx context.Context) error {
		return evo.File{Path: path, Content: evo.Bytes("same"), Mode: 0o644}.Write(ctx)
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatalf("an already-satisfied Write replaced the file")
	}
}

func TestFileWritePublishesAtomicallyLeavingNoSiblings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	mustWrite(t, path, []byte("old"), 0o644)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	err = contractRun(t, evo.Config{}, func(ctx context.Context) error {
		return evo.File{Path: path, Content: evo.Bytes("new")}.Write(ctx)
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Fatalf("Write edited the file in place; want replacement by a new file")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "f.txt" {
		t.Fatalf("Write left siblings beside the destination: %v", entries)
	}
}

func TestFileWriteIsVerifiedAfterCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	f := evo.File{Path: path, Content: evo.Bytes("desired"), Mode: 0o600}
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		if err := f.Write(ctx); err != nil {
			return err
		}
		return f.Verify(ctx)
	})
	if err != nil {
		t.Fatalf("Write then Verify: %v", err)
	}
}

func TestFileReadReturnsTheBytesOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	mustWrite(t, path, []byte("on disk"), 0o644)
	var got []byte
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		var err error
		// Content is desired state; Read reports observed state.
		got, err = evo.File{Path: path, Content: evo.Bytes("desired")}.Read(ctx)
		return err
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != "on disk" {
		t.Fatalf("Read = %q, want %q", got, "on disk")
	}
}

func TestFileReadOfAMissingFileIsNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.txt")
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		_, err := evo.File{Path: path}.Read(ctx)
		return err
	})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Read of a missing file = %v, want fs.ErrNotExist", err)
	}
}

func TestFileVerifyComparesDiskToDesiredState(t *testing.T) {
	cases := []struct {
		name     string
		disk     []byte // nil: path is missing
		diskMode fs.FileMode
		file     evo.File
		mismatch bool
	}{
		{"matching content", []byte("x"), 0o644, evo.File{Content: evo.Bytes("x")}, false},
		{"matching content and mode", []byte("x"), 0o600, evo.File{Content: evo.Bytes("x"), Mode: 0o600}, false},
		{"differing content", []byte("y"), 0o644, evo.File{Content: evo.Bytes("x")}, true},
		{"differing mode", []byte("x"), 0o644, evo.File{Content: evo.Bytes("x"), Mode: 0o600}, true},
		{"missing file", nil, 0, evo.File{Content: evo.Bytes("x")}, true},
		{"zero Mode does not check the disk mode", []byte("x"), 0o600, evo.File{Content: evo.Bytes("x")}, false},
		{"empty Bytes matches an empty file", []byte{}, 0o644, evo.File{Content: evo.Bytes("")}, false},
		{"empty Bytes against non-empty file", []byte("x"), 0o644, evo.File{Content: evo.Bytes("")}, true},
		{"non-empty Bytes against empty file", []byte{}, 0o644, evo.File{Content: evo.Bytes("x")}, true},
		{"longer disk content with matching prefix", []byte("xy"), 0o644, evo.File{Content: evo.Bytes("x")}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "f.txt")
			if tc.disk != nil {
				mustWrite(t, path, tc.disk, tc.diskMode)
			}
			f := tc.file
			f.Path = path
			err := contractRun(t, evo.Config{}, f.Verify)
			if tc.mismatch && !errors.Is(err, evo.ErrVerifyMismatch) {
				t.Fatalf("Verify = %v, want ErrVerifyMismatch", err)
			}
			if !tc.mismatch && err != nil {
				t.Fatalf("Verify = %v, want nil", err)
			}
			if tc.disk == nil {
				if _, statErr := os.Lstat(path); !errors.Is(statErr, fs.ErrNotExist) {
					t.Fatalf("Verify created the file it was checking")
				}
			}
		})
	}
}

func TestFileEqualComparesObservedBytes(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"identical bytes", "same", "same", true},
		{"different bytes", "one", "two", false},
		{"both empty", "", "", true},
		{"empty against non-empty", "", "x", false},
		{"prefix is not equal", "ab", "abc", false},
		{"same length one byte apart", "abc", "abd", false},
		{"binary with NUL bytes", "a\x00b\xff", "a\x00b\xff", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
			mustWrite(t, a, []byte(tc.a), 0o644)
			mustWrite(t, b, []byte(tc.b), 0o600) // mode is not content
			var got bool
			err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
				var err error
				got, err = evo.File{Path: a}.Equal(ctx, evo.File{Path: b})
				return err
			})
			if err != nil {
				t.Fatalf("Equal: %v", err)
			}
			if got != tc.want {
				t.Fatalf("Equal = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFileWriteRequiresAPath(t *testing.T) {
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		return evo.File{Content: evo.Bytes("x")}.Write(ctx)
	})
	if !errors.Is(err, evo.ErrPathMissing) {
		t.Fatalf("Write with empty Path = %v, want ErrPathMissing", err)
	}
}

func TestFileWriteRequiresATaskContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	err := evo.File{Path: path, Content: evo.Bytes("x")}.Write(context.Background())
	if !errors.Is(err, evo.ErrNoTaskContext) {
		t.Fatalf("Write outside a Task = %v, want ErrNoTaskContext", err)
	}
	if _, statErr := os.Lstat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("Write outside a Task still created the file")
	}
}

func TestFileWriteUnderDryRunMutatesNothing(t *testing.T) {
	dir := t.TempDir()
	existing, missing := filepath.Join(dir, "existing.txt"), filepath.Join(dir, "missing.txt")
	mustWrite(t, existing, []byte("old"), 0o644)
	err := contractRun(t, evo.Config{DryRun: true}, func(ctx context.Context) error {
		if err := (evo.File{Path: existing, Content: evo.Bytes("new")}).Write(ctx); err != nil {
			return err
		}
		return evo.File{Path: missing, Content: evo.Bytes("new")}.Write(ctx)
	})
	if err != nil {
		t.Fatalf("dry-run Write: %v", err)
	}
	if got, _ := os.ReadFile(existing); string(got) != "old" {
		t.Fatalf("dry-run Write changed an existing file to %q", got)
	}
	if _, statErr := os.Lstat(missing); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("dry-run Write created a file")
	}
}

// Sibling Tasks write one destination with no Lock, Unlock, or Resource in
// caller code; every Write succeeds and the file holds exactly one whole
// candidate.
func TestFileWriteNeedsNoCallerCoordination(t *testing.T) {
	const writers = 8
	path := filepath.Join(t.TempDir(), "shared.txt")
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir(),
	})
	group := out.Group("writers")
	errs := make([]error, writers)
	candidates := map[string]bool{}
	for i := range writers {
		body := fmt.Sprintf("writer %d\n%s", i, string(make([]byte, 64<<10)))
		candidates[body] = true
		group.Task(fmt.Sprintf("writer %d", i)).Define(func(ctx context.Context) error {
			errs[i] = evo.File{Path: path, Content: evo.Bytes(body)}.Write(ctx)
			return errs[i]
		})
	}
	_ = group.Wait()
	_ = out.Finish()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("writer %d: %v", i, err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !candidates[string(got)] {
		t.Fatalf("destination holds %d bytes that match no writer's whole content", len(got))
	}
}

func TestFileEqualOfAFileWithItself(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	mustWrite(t, path, []byte("x"), 0o644)
	var got bool
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		var err error
		got, err = evo.File{Path: path}.Equal(ctx, evo.File{Path: path})
		return err
	})
	if err != nil || !got {
		t.Fatalf("Equal(self) = %v, %v; want true, nil", got, err)
	}
}

func TestFileWriteChangesModeAndKeepsContentWhenContentAlreadyMatches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	mustWrite(t, path, []byte("same"), 0o644)
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		return evo.File{Path: path, Content: evo.Bytes("same"), Mode: 0o600}.Write(ctx)
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
	if got, _ := os.ReadFile(path); string(got) != "same" {
		t.Fatalf("content = %q after a mode-only change", got)
	}
}

func TestFileBytesEstablishesARegularFileEvenWhenEmpty(t *testing.T) {
	for name, content := range map[string]evo.FileContent{"nil": evo.Bytes([]byte(nil)), "empty string": evo.Bytes(""), "empty slice": evo.Bytes([]byte{})} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "f")
			mustWrite(t, path, []byte("old"), 0o644)
			err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
				return evo.File{Path: path, Content: content}.Write(ctx)
			})
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			info, statErr := os.Lstat(path)
			if statErr != nil {
				t.Fatalf("empty Bytes removed the file instead of emptying it: %v", statErr)
			}
			if !info.Mode().IsRegular() || info.Size() != 0 {
				t.Fatalf("path is %v size %d, want an empty regular file", info.Mode(), info.Size())
			}
		})
	}
}

func TestFileBytesAcceptsStringAndByteSliceIdentically(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		if err := (evo.File{Path: a, Content: evo.Bytes("same bytes")}).Write(ctx); err != nil {
			return err
		}
		return evo.File{Path: b, Content: evo.Bytes([]byte("same bytes"))}.Write(ctx)
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	ga, _ := os.ReadFile(a)
	gb, _ := os.ReadFile(b)
	if string(ga) != "same bytes" || string(gb) != "same bytes" {
		t.Fatalf("string/[]byte Bytes disagree: %q vs %q", ga, gb)
	}
}

// A nil Content declares nothing; it never means "make this path absent".
func TestFileWriteWithNilContentIsErrContentMissingAndTouchesNothing(t *testing.T) {
	dir := t.TempDir()
	existing, missing := filepath.Join(dir, "existing"), filepath.Join(dir, "missing")
	mustWrite(t, existing, []byte("keep"), 0o644)
	for name, path := range map[string]string{"existing": existing, "missing": missing} {
		t.Run(name, func(t *testing.T) {
			err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
				return evo.File{Path: path, Mode: 0o600}.Write(ctx)
			})
			if !errors.Is(err, evo.ErrContentMissing) {
				t.Fatalf("Write with nil Content = %v, want ErrContentMissing", err)
			}
		})
	}
	if got, err := os.ReadFile(existing); err != nil || string(got) != "keep" {
		t.Fatalf("nil-Content Write changed the existing file: %q, %v", got, err)
	}
	if info, _ := os.Stat(existing); info.Mode().Perm() != 0o644 {
		t.Fatalf("nil-Content Write changed the mode to %v", info.Mode().Perm())
	}
	if _, err := os.Lstat(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("nil-Content Write created the missing path")
	}
}

func TestFileEveryMethodRequiresAPath(t *testing.T) {
	calls := map[string]func(ctx context.Context) error{
		"Read":     func(ctx context.Context) error { _, err := evo.File{}.Read(ctx); return err },
		"Verify":   evo.File{Content: evo.Bytes("x")}.Verify,
		"Checksum": func(ctx context.Context) error { _, err := evo.File{}.Checksum(ctx); return err },
		"Equal": func(ctx context.Context) error {
			_, err := evo.File{}.Equal(ctx, evo.File{Path: filepath.Join(t.TempDir(), "x")})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := contractRun(t, evo.Config{}, call); !errors.Is(err, evo.ErrPathMissing) {
				t.Fatalf("%s with empty Path = %v, want ErrPathMissing", name, err)
			}
		})
	}
}

func TestFileChecksumIsLowercaseHexSHA256OfTheBytes(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string // independently computed SHA-256 of data
	}{
		{"abc", []byte("abc"), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
		{"empty file", []byte{}, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"binary", []byte{0, 1, 2, 0xff, 0xfe}, hexSum([]byte{0, 1, 2, 0xff, 0xfe})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "f")
			mustWrite(t, path, tc.data, 0o644)
			var got string
			err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
				var err error
				// Declared Content is desired state; Checksum reports the observed bytes.
				got, err = evo.File{Path: path, Content: evo.Bytes("ignored")}.Checksum(ctx)
				return err
			})
			if err != nil {
				t.Fatalf("Checksum: %v", err)
			}
			if got != tc.want {
				t.Fatalf("Checksum = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFileChecksumIgnoresModeAndTracksContent(t *testing.T) {
	dir := t.TempDir()
	a, b, c := filepath.Join(dir, "a"), filepath.Join(dir, "b"), filepath.Join(dir, "c")
	mustWrite(t, a, []byte("same"), 0o644)
	mustWrite(t, b, []byte("same"), 0o755)
	mustWrite(t, c, []byte("sbme"), 0o644)
	sums := map[string]string{}
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		for name, path := range map[string]string{"a": a, "b": b, "c": c} {
			sum, err := evo.File{Path: path}.Checksum(ctx)
			if err != nil {
				return err
			}
			sums[name] = sum
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Checksum: %v", err)
	}
	if sums["a"] != sums["b"] {
		t.Fatalf("mode changed the checksum: %q vs %q", sums["a"], sums["b"])
	}
	if sums["a"] == sums["c"] {
		t.Fatalf("a one-byte difference did not change the checksum")
	}
}

func TestFileChecksumOfAMissingFileIsNotExist(t *testing.T) {
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		_, err := evo.File{Path: filepath.Join(t.TempDir(), "missing")}.Checksum(ctx)
		return err
	})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Checksum of a missing file = %v, want fs.ErrNotExist", err)
	}
}

// Verify of a missing path with declared Content is a mismatch, and a
// mismatching Verify only reports: it never repairs.
func TestFileVerifyNeverRepairs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	mustWrite(t, path, []byte("drifted"), 0o600)
	err := contractRun(t, evo.Config{}, evo.File{Path: path, Content: evo.Bytes("desired"), Mode: 0o644}.Verify)
	if !errors.Is(err, evo.ErrVerifyMismatch) {
		t.Fatalf("Verify = %v, want ErrVerifyMismatch", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "drifted" {
		t.Fatalf("Verify repaired the content to %q", got)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("Verify repaired the mode to %v", info.Mode().Perm())
	}
}

func hexSum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
