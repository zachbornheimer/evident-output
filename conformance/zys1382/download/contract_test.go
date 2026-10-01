// Package download_test is the ZYS-1382 execution contract for evo.Download,
// the File content producer that fetches a URL and enforces Integrity before
// anything reaches the destination. Names and open-syntax choices:
// docs/zys-1382/contract-decisions.md.
package download_test

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

var _ evo.FileContent = evo.Download{}

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

var payload = []byte("tarball bytes of a locked package\n")

func sri(algo string, data []byte) string {
	var sum []byte
	switch algo {
	case "sha512":
		s := sha512.Sum512(data)
		sum = s[:]
	case "sha384":
		s := sha512.Sum384(data)
		sum = s[:]
	case "sha256":
		s := sha256.Sum256(data)
		sum = s[:]
	}
	return algo + "-" + base64.StdEncoding.EncodeToString(sum)
}

// serve returns a server that answers every GET with data and counts hits.
func serve(t *testing.T, data []byte) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestDownloadIsAPlainStructLiteral(t *testing.T) {
	d := evo.Download{URL: "https://registry.invalid/pkg.tgz", Integrity: sri("sha512", payload)}
	f := evo.File{Path: "pkg.tgz", Content: d}
	if f.Content == nil || d.URL == "" || d.Integrity == "" {
		t.Fatalf("Download literal did not round-trip: %+v", f)
	}
}

func TestDownloadPublishesWhenIntegrityMatches(t *testing.T) {
	sha1Sum := sha1.Sum(payload)
	sha256Sum := sha256.Sum256(payload)
	cases := []struct {
		name      string
		integrity string
	}{
		{"SRI sha512", sri("sha512", payload)},
		{"SRI sha384", sri("sha384", payload)},
		{"SRI sha256", sri("sha256", payload)},
		{"plain hex sha256", hex.EncodeToString(sha256Sum[:])},
		{"plain hex sha1", hex.EncodeToString(sha1Sum[:])},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := serve(t, payload)
			path := filepath.Join(t.TempDir(), "pkg.tgz")
			f := evo.File{Path: path, Content: evo.Download{URL: srv.URL + "/pkg.tgz", Integrity: tc.integrity}}
			if err := contractRun(t, evo.Config{}, f.Write); err != nil {
				t.Fatalf("Write: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(payload) {
				t.Fatalf("destination = %q, want the served payload", got)
			}
		})
	}
}

func TestDownloadIntegrityMismatchNeverPublishes(t *testing.T) {
	other := []byte("not the bytes the lockfile promised\n")
	cases := []struct {
		name      string
		integrity string
	}{
		{"SRI sha512 of other bytes", sri("sha512", other)},
		{"SRI sha256 of other bytes", sri("sha256", other)},
		{"hex sha256 of other bytes", func() string { s := sha256.Sum256(other); return hex.EncodeToString(s[:]) }()},
		{"unparseable integrity", "md5-not-supported"},
		{"empty integrity", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := serve(t, payload)
			dir := t.TempDir()
			path := filepath.Join(dir, "pkg.tgz")
			f := evo.File{Path: path, Content: evo.Download{URL: srv.URL + "/pkg.tgz", Integrity: tc.integrity}}
			err := contractRun(t, evo.Config{}, f.Write)
			if !errors.Is(err, evo.ErrIntegrityMismatch) {
				t.Fatalf("Write = %v, want ErrIntegrityMismatch", err)
			}
			if _, statErr := os.Lstat(path); !errors.Is(statErr, fs.ErrNotExist) {
				t.Fatalf("a failed integrity check still published the destination")
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 0 {
				t.Fatalf("a failed download left temporaries beside the destination: %v", entries)
			}
		})
	}
}

func TestDownloadIntegrityMismatchKeepsTheExistingDestination(t *testing.T) {
	srv, _ := serve(t, payload)
	path := filepath.Join(t.TempDir(), "pkg.tgz")
	if err := os.WriteFile(path, []byte("previous good"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := evo.File{Path: path, Content: evo.Download{URL: srv.URL + "/pkg.tgz", Integrity: sri("sha512", []byte("wrong"))}}
	if err := contractRun(t, evo.Config{}, f.Write); !errors.Is(err, evo.ErrIntegrityMismatch) {
		t.Fatalf("Write = %v, want ErrIntegrityMismatch", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "previous good" {
		t.Fatalf("destination = %q after a rejected download, want it untouched", got)
	}
}

func TestDownloadAlreadySatisfiedDoesNotFetch(t *testing.T) {
	srv, hits := serve(t, payload)
	path := filepath.Join(t.TempDir(), "pkg.tgz")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	f := evo.File{Path: path, Content: evo.Download{URL: srv.URL + "/pkg.tgz", Integrity: sri("sha512", payload)}}
	if err := contractRun(t, evo.Config{}, f.Write); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("Write fetched %d times for a destination that already matched Integrity", n)
	}
}

func TestDownloadReplacesAStaleDestination(t *testing.T) {
	srv, hits := serve(t, payload)
	path := filepath.Join(t.TempDir(), "pkg.tgz")
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := evo.File{Path: path, Content: evo.Download{URL: srv.URL + "/pkg.tgz", Integrity: sri("sha512", payload)}}
	if err := contractRun(t, evo.Config{}, f.Write); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != string(payload) {
		t.Fatalf("destination = %q, want the served payload", got)
	}
	if hits.Load() != 1 {
		t.Fatalf("Write fetched %d times, want exactly once", hits.Load())
	}
}

func TestDownloadAppliesMode(t *testing.T) {
	srv, _ := serve(t, payload)
	path := filepath.Join(t.TempDir(), "tool")
	f := evo.File{Path: path, Mode: 0o755, Content: evo.Download{URL: srv.URL + "/tool", Integrity: sri("sha512", payload)}}
	if err := contractRun(t, evo.Config{}, f.Write); err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v, want 0755", info.Mode().Perm())
	}
}

func TestDownloadHTTPFailureNeverPublishes(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{"404", http.StatusNotFound},
		{"500", http.StatusInternalServerError},
		{"403", http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte("error body"))
			}))
			t.Cleanup(srv.Close)
			path := filepath.Join(t.TempDir(), "pkg.tgz")
			f := evo.File{Path: path, Content: evo.Download{URL: srv.URL + "/pkg.tgz", Integrity: sri("sha512", payload)}}
			err := contractRun(t, evo.Config{}, f.Write)
			if !errors.Is(err, evo.ErrDownloadFailed) {
				t.Fatalf("Write = %v, want ErrDownloadFailed", err)
			}
			if _, statErr := os.Lstat(path); !errors.Is(statErr, fs.ErrNotExist) {
				t.Fatalf("an HTTP %d response was published as the destination", tc.status)
			}
		})
	}
}

func TestDownloadRequiresAURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pkg.tgz")
	f := evo.File{Path: path, Content: evo.Download{Integrity: sri("sha512", payload)}}
	if err := contractRun(t, evo.Config{}, f.Write); !errors.Is(err, evo.ErrDownloadURLMissing) {
		t.Fatalf("Write = %v, want ErrDownloadURLMissing", err)
	}
}

func TestDownloadVerifyChecksIntegrityWithoutFetching(t *testing.T) {
	srv, hits := serve(t, payload)
	path := filepath.Join(t.TempDir(), "pkg.tgz")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	f := evo.File{Path: path, Content: evo.Download{URL: srv.URL + "/pkg.tgz", Integrity: sri("sha512", payload)}}
	if err := contractRun(t, evo.Config{}, f.Verify); err != nil {
		t.Fatalf("Verify of a matching destination = %v", err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := contractRun(t, evo.Config{}, f.Verify); !errors.Is(err, evo.ErrVerifyMismatch) {
		t.Fatalf("Verify of a tampered destination = %v, want ErrVerifyMismatch", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("Verify fetched %d times; Integrity is enough", hits.Load())
	}
}

func TestDownloadUnderDryRunNeitherFetchesNorPublishes(t *testing.T) {
	srv, hits := serve(t, payload)
	path := filepath.Join(t.TempDir(), "pkg.tgz")
	f := evo.File{Path: path, Content: evo.Download{URL: srv.URL + "/pkg.tgz", Integrity: sri("sha512", payload)}}
	if err := contractRun(t, evo.Config{DryRun: true}, f.Write); err != nil {
		t.Fatalf("dry-run Write: %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("dry-run Write fetched %d times", hits.Load())
	}
	if _, statErr := os.Lstat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("dry-run Write published the destination")
	}
}

func TestDownloadHonorsContextCancellation(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	path := filepath.Join(t.TempDir(), "pkg.tgz")
	f := evo.File{Path: path, Content: evo.Download{URL: srv.URL + "/pkg.tgz", Integrity: sri("sha512", payload)}}
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		ctx, cancel := context.WithCancel(ctx)
		go cancel()
		return f.Write(ctx)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Write = %v, want context.Canceled", err)
	}
	if _, statErr := os.Lstat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("a cancelled download published the destination")
	}
}

func TestDownloadWriteLeavesOnlyTheDestinationAndVerifies(t *testing.T) {
	srv, _ := serve(t, payload)
	dir := t.TempDir()
	path := filepath.Join(dir, "pkg.tgz")
	f := evo.File{Path: path, Content: evo.Download{URL: srv.URL + "/pkg.tgz", Integrity: sri("sha512", payload)}}
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		if err := f.Write(ctx); err != nil {
			return err
		}
		return f.Verify(ctx)
	})
	if err != nil {
		t.Fatalf("Write then Verify: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "pkg.tgz" {
		t.Fatalf("destination parent holds %v, want only pkg.tgz", entries)
	}
}

func TestDownloadMismatchLeavesOnlyTheExistingDestination(t *testing.T) {
	srv, _ := serve(t, payload)
	dir := t.TempDir()
	plant(t, dir, map[string]string{"pkg.tgz": "previous good"})
	f := evo.File{Path: filepath.Join(dir, "pkg.tgz"), Content: evo.Download{URL: srv.URL, Integrity: sri("sha512", []byte("wrong"))}}
	if err := contractRun(t, evo.Config{}, f.Write); !errors.Is(err, evo.ErrIntegrityMismatch) {
		t.Fatalf("Write = %v, want ErrIntegrityMismatch", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("a rejected download left temporaries beside the destination: %v", entries)
	}
}

func TestDownloadFollowsRedirectsToASuccessfulResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new", http.StatusFound)
	})
	mux.HandleFunc("/new", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	path := filepath.Join(t.TempDir(), "pkg.tgz")
	f := evo.File{Path: path, Content: evo.Download{URL: srv.URL + "/old", Integrity: sri("sha512", payload)}}
	if err := contractRun(t, evo.Config{}, f.Write); err != nil {
		t.Fatalf("Write through a redirect: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != string(payload) {
		t.Fatalf("destination = %q, want the redirected payload", got)
	}
}

func TestDownloadRedirectToFailureNeverPublishes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/gone", http.StatusFound)
	})
	mux.HandleFunc("/gone", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(payload)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	path := filepath.Join(t.TempDir(), "pkg.tgz")
	f := evo.File{Path: path, Content: evo.Download{URL: srv.URL + "/old", Integrity: sri("sha512", payload)}}
	if err := contractRun(t, evo.Config{}, f.Write); !errors.Is(err, evo.ErrDownloadFailed) {
		t.Fatalf("Write = %v, want ErrDownloadFailed", err)
	}
	if _, statErr := os.Lstat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("a redirect to a 404 was published")
	}
}

func TestDownloadOutsideATaskIsRefusedBeforeAnyRequest(t *testing.T) {
	srv, hits := serve(t, payload)
	path := filepath.Join(t.TempDir(), "pkg.tgz")
	f := evo.File{Path: path, Content: evo.Download{URL: srv.URL, Integrity: sri("sha512", payload)}}
	if err := f.Write(context.Background()); !errors.Is(err, evo.ErrNoTaskContext) {
		t.Fatalf("Write outside a Task = %v, want ErrNoTaskContext", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("Write outside a Task fetched %d times", hits.Load())
	}
	if _, statErr := os.Lstat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("Write outside a Task created the destination")
	}
}

func TestDownloadRequiresAPath(t *testing.T) {
	srv, hits := serve(t, payload)
	f := evo.File{Content: evo.Download{URL: srv.URL, Integrity: sri("sha512", payload)}}
	if err := contractRun(t, evo.Config{}, f.Write); !errors.Is(err, evo.ErrPathMissing) {
		t.Fatalf("Write with an empty Path = %v, want ErrPathMissing", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("Write with an empty Path fetched %d times", hits.Load())
	}
}

func TestDownloadMissingURLLeavesTheDestinationUntouched(t *testing.T) {
	dir := t.TempDir()
	plant(t, dir, map[string]string{"pkg.tgz": "previous good"})
	f := evo.File{Path: filepath.Join(dir, "pkg.tgz"), Content: evo.Download{Integrity: sri("sha512", payload)}}
	if err := contractRun(t, evo.Config{}, f.Write); !errors.Is(err, evo.ErrDownloadURLMissing) {
		t.Fatalf("Write = %v, want ErrDownloadURLMissing", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "pkg.tgz")); string(got) != "previous good" {
		t.Fatalf("destination = %q, want it untouched", got)
	}
}

func TestDownloadVerifyOfAMissingDestinationIsAMismatch(t *testing.T) {
	srv, hits := serve(t, payload)
	f := evo.File{Path: filepath.Join(t.TempDir(), "pkg.tgz"), Content: evo.Download{URL: srv.URL, Integrity: sri("sha512", payload)}}
	if err := contractRun(t, evo.Config{}, f.Verify); !errors.Is(err, evo.ErrVerifyMismatch) {
		t.Fatalf("Verify of a missing destination = %v, want ErrVerifyMismatch", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("Verify fetched %d times", hits.Load())
	}
}

func TestDownloadWriteIsANoOpWhenSatisfied(t *testing.T) {
	srv, _ := serve(t, payload)
	path := filepath.Join(t.TempDir(), "pkg.tgz")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	f := evo.File{Path: path, Content: evo.Download{URL: srv.URL, Integrity: sri("sha512", payload)}}
	if err := contractRun(t, evo.Config{}, f.Write); err != nil {
		t.Fatalf("Write: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatalf("Write replaced a destination that already satisfied Integrity (new inode)")
	}
}

func TestDownloadAppliesModeToASatisfiedDestination(t *testing.T) {
	srv, hits := serve(t, payload)
	path := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	f := evo.File{Path: path, Mode: 0o755, Content: evo.Download{URL: srv.URL, Integrity: sri("sha512", payload)}}
	if err := contractRun(t, evo.Config{}, f.Write); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v, want 0755", info.Mode().Perm())
	}
	if hits.Load() != 0 {
		t.Fatalf("Write fetched %d times to change only the mode", hits.Load())
	}
}

func TestDownloadPreservesTheExistingModeWhenModeIsZero(t *testing.T) {
	srv, _ := serve(t, payload)
	path := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	f := evo.File{Path: path, Content: evo.Download{URL: srv.URL, Integrity: sri("sha512", payload)}}
	if err := contractRun(t, evo.Config{}, f.Write); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want the existing 0600 preserved", info.Mode().Perm())
	}
}

// The slow part (the fetch) happens outside the exclusive destination lock.
// While a download is in flight, a sibling Task writes the same destination;
// that write must finish without waiting for the fetch. The download's own
// outcome is not pinned (it may win or report staleness), but the destination
// always ends holding one whole content and no temporaries.
func TestDownloadDoesNotHoldTheDestinationLockWhileFetching(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var startOnce, releaseOnce sync.Once
	releaseFetch := func() { releaseOnce.Do(func() { close(release) }) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		startOnce.Do(func() { close(started) })
		<-release
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(releaseFetch)

	dir := t.TempDir()
	dest := filepath.Join(dir, "pkg.tgz")
	var writeErr error
	siblingDone := make(chan struct{})
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir(), MaxConcurrency: 4})
	defer func() { _ = out.Close() }()
	group := out.Group("writers")
	group.Task("download").Define(evo.File{Path: dest, Content: evo.Download{URL: srv.URL, Integrity: sri("sha512", payload)}}.Write)
	group.Task("sibling").Define(func(ctx context.Context) error {
		defer close(siblingDone)
		select {
		case <-started:
		case <-time.After(10 * time.Second):
			return errors.New("the download never started")
		}
		writeErr = evo.File{Path: dest, Content: evo.Bytes("local edit")}.Write(ctx)
		return writeErr
	})
	go func() {
		select {
		case <-siblingDone:
		case <-time.After(10 * time.Second):
			t.Errorf("a sibling write blocked behind an in-flight download: the fetch holds the destination lock")
		}
		releaseFetch()
	}()
	_ = group.Wait()
	_ = out.Finish()
	if writeErr != nil {
		t.Fatalf("sibling Write during a fetch = %v", writeErr)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != string(payload) && string(got) != "local edit" {
		t.Fatalf("destination = %q, want exactly one writer's whole content", got)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("destination parent holds %v, want only the destination", entries)
	}
}
