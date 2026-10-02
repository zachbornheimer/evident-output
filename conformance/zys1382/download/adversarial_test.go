package download_test

// Adversarial hardening tests for evo.Download. Each test is named for the
// weakness it guards; sources are in docs/zys-1382/adversarial-research.md.

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

const advLiveness = 10 * time.Second

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

func advSHA512(data []byte) string {
	sum := sha512.Sum512(data)
	return "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
}

func advSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
}

// advServer serves body at every path and counts requests.
func advServer(tb testing.TB, handler http.HandlerFunc) (*httptest.Server, *atomic.Int64) {
	tb.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	tb.Cleanup(srv.Close)
	return srv, &hits
}

func advStatic(body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }
}

func advDownload(tb testing.TB, ctxTimeout time.Duration, dest, rawURL, integrity string) error {
	tb.Helper()
	return advRun(tb, evo.Config{}, func(ctx context.Context) error {
		if ctxTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, ctxTimeout)
			defer cancel()
		}
		return evo.File{Path: dest, Content: evo.Download{URL: rawURL, Integrity: integrity}}.Write(ctx)
	})
}

func advAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("%s was published (stat err %v)", path, err)
	}
}

func advDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("%s holds leftover entries, first %q", dir, entries[0].Name())
	}
}

func advTimed(t *testing.T, what string, fn func() error) error {
	t.Helper()
	start := time.Now()
	err := fn()
	if elapsed := time.Since(start); elapsed > advLiveness {
		t.Fatalf("%s took %v; the deadline was not honored", what, elapsed)
	}
	return err
}

func TestAdversarial_IntegrityMismatchNeverPublishes(t *testing.T) {
	srv, _ := advServer(t, advStatic([]byte("evil")))
	dest := filepath.Join(t.TempDir(), "pkg.tgz")
	if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := advDownload(t, 0, dest, srv.URL, advSHA512([]byte("good"))); !errors.Is(err, evo.ErrIntegrityMismatch) {
		t.Fatalf("mismatching integrity = %v, want ErrIntegrityMismatch", err)
	}
	if got, _ := os.ReadFile(dest); string(got) != "old" {
		t.Fatalf("integrity mismatch replaced the destination with %q", got)
	}
}

// The integrity matches the bytes received, but the transfer ended early.
func TestAdversarial_TruncatedBodyRejected(t *testing.T) {
	prefix := []byte("0123456789")
	srv, _ := advServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write(prefix)
	})
	dir := t.TempDir()
	dest := filepath.Join(dir, "f")
	if err := advDownload(t, 0, dest, srv.URL, advSHA512(prefix)); err == nil {
		t.Fatal("a truncated transfer was published")
	}
	advDirEmpty(t, dir)
}

func TestAdversarial_StalledServerHonorsDeadline(t *testing.T) {
	srv, _ := advServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	dest := filepath.Join(t.TempDir(), "f")
	err := advTimed(t, "Download from a stalled server", func() error {
		return advDownload(t, 500*time.Millisecond, dest, srv.URL, advSHA512(make([]byte, 100)))
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stalled Download = %v, want context.DeadlineExceeded", err)
	}
	advAbsent(t, dest)
}

// TUF "endless data": the body never ends. The deadline stops it and no
// partial temp file survives.
func TestAdversarial_EndlessBodyHonorsDeadlineAndLeaksNoTemp(t *testing.T) {
	chunk := make([]byte, 64<<10)
	srv, _ := advServer(t, func(w http.ResponseWriter, r *http.Request) {
		for r.Context().Err() == nil {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	})
	dir := t.TempDir()
	err := advTimed(t, "Download of an endless body", func() error {
		return advDownload(t, time.Second, filepath.Join(dir, "f"), srv.URL, advSHA512(chunk))
	})
	if err == nil {
		t.Fatal("endless Download succeeded")
	}
	advDirEmpty(t, dir)
}

func TestAdversarial_MissingIntegrityRejected(t *testing.T) {
	srv, hits := advServer(t, advStatic([]byte("anything")))
	dest := filepath.Join(t.TempDir(), "f")
	if err := advDownload(t, 0, dest, srv.URL, ""); !errors.Is(err, evo.ErrIntegrityMismatch) {
		t.Fatalf("Download without Integrity = %v, want ErrIntegrityMismatch", err)
	}
	advAbsent(t, dest)
	if hits.Load() != 0 {
		t.Fatalf("Download without Integrity still made %d request(s)", hits.Load())
	}
}

func TestAdversarial_MalformedIntegrityRejected(t *testing.T) {
	body := []byte("payload")
	srv, _ := advServer(t, advStatic(body))
	for _, integrity := range []string{
		"sha512-",
		"sha512-%%%not-base64%%%",
		"sha512-" + base64.StdEncoding.EncodeToString([]byte("short")),
		"sha999-" + strings.TrimPrefix(advSHA512(body), "sha512-"),
		"   ",
		"zzzz" + strings.Repeat("0", 60),
	} {
		dest := filepath.Join(t.TempDir(), "f")
		if err := advDownload(t, 0, dest, srv.URL, integrity); !errors.Is(err, evo.ErrIntegrityMismatch) {
			t.Fatalf("malformed integrity %q = %v, want ErrIntegrityMismatch", integrity, err)
		}
		advAbsent(t, dest)
	}
}

func TestAdversarial_WeakAlgorithmRejected(t *testing.T) {
	body := []byte("payload")
	srv, _ := advServer(t, advStatic(body))
	sum := md5.Sum(body)
	dest := filepath.Join(t.TempDir(), "f")
	if err := advDownload(t, 0, dest, srv.URL, "md5-"+base64.StdEncoding.EncodeToString(sum[:])); !errors.Is(err, evo.ErrIntegrityMismatch) {
		t.Fatalf("md5 integrity = %v, want ErrIntegrityMismatch", err)
	}
	advAbsent(t, dest)
}

// W3C SRI: the strongest algorithm listed must match; a matching weaker
// hash never rescues a mismatching stronger one.
func TestAdversarial_StrongestAlgorithmWins(t *testing.T) {
	body := []byte("payload")
	srv, _ := advServer(t, advStatic(body))
	wrong := []byte("other")

	dest := filepath.Join(t.TempDir(), "f")
	if err := advDownload(t, 0, dest, srv.URL, advSHA256(body)+" "+advSHA512(wrong)); !errors.Is(err, evo.ErrIntegrityMismatch) {
		t.Fatalf("a matching sha256 overrode a mismatching sha512: %v", err)
	}
	advAbsent(t, dest)

	dest = filepath.Join(t.TempDir(), "f")
	if err := advDownload(t, 0, dest, srv.URL, advSHA256(wrong)+" "+advSHA512(body)); err != nil {
		t.Fatalf("matching strongest hash rejected: %v", err)
	}
}

func TestAdversarial_HexDigestLengthEnforced(t *testing.T) {
	body := []byte("payload")
	srv, _ := advServer(t, advStatic(body))
	sum := sha256.Sum256(body)
	full := hex.EncodeToString(sum[:])
	for _, integrity := range []string{full[:63], full[:32], full + "00"} {
		dest := filepath.Join(t.TempDir(), "f")
		if err := advDownload(t, 0, dest, srv.URL, integrity); !errors.Is(err, evo.ErrIntegrityMismatch) {
			t.Fatalf("hex digest of length %d = %v, want ErrIntegrityMismatch", len(integrity), err)
		}
		advAbsent(t, dest)
	}
}

// The error page itself matches the integrity; a non-2xx status still never
// publishes.
func TestAdversarial_NonSuccessStatusNeverPublishes(t *testing.T) {
	page := []byte("not found")
	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		srv, _ := advServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write(page)
		})
		dest := filepath.Join(t.TempDir(), "f")
		if err := advDownload(t, 0, dest, srv.URL, advSHA512(page)); !errors.Is(err, evo.ErrDownloadFailed) {
			t.Fatalf("status %d = %v, want ErrDownloadFailed", status, err)
		}
		advAbsent(t, dest)
	}
}

func TestAdversarial_RedirectLoopBounded(t *testing.T) {
	srv, _ := advServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
	})
	dest := filepath.Join(t.TempDir(), "f")
	err := advTimed(t, "Download through a redirect loop", func() error {
		return advDownload(t, 0, dest, srv.URL+"/loop", advSHA512(nil))
	})
	if err == nil {
		t.Fatal("redirect loop succeeded")
	}
	advAbsent(t, dest)
}

// advSyncBuffer is a goroutine-safe sink for rendered output.
type advSyncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *advSyncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *advSyncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestAdversarial_URLCredentialsRedactedInErrors(t *testing.T) {
	const secret = "s3kr3t-token"
	srv, _ := advServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword("user", secret)
	var rendered advSyncBuffer
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: &rendered, Stderr: &rendered, StateDir: t.TempDir()})
	dest := filepath.Join(t.TempDir(), "f")
	err = out.Task("download").Define(func(ctx context.Context) error {
		return evo.File{Path: dest, Content: evo.Download{URL: u.String(), Integrity: advSHA512(nil)}}.Write(ctx)
	}).Wait()
	_ = out.Finish()
	_ = out.Close()
	if err == nil {
		t.Fatal("a 401 response was published")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaks URL credentials: %v", err)
	}
	if strings.Contains(rendered.String(), secret) {
		t.Fatalf("rendered output leaks URL credentials")
	}
}

func TestAdversarial_ConcurrentSameDestinationPublishesOnce(t *testing.T) {
	body := []byte(strings.Repeat("payload", 10_000))
	srv, _ := advServer(t, advStatic(body))
	dir := t.TempDir()
	dest := filepath.Join(dir, "pkg.tgz")
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()})
	defer func() { _ = out.Close() }()
	group := out.Group("downloads")
	file := evo.File{Path: dest, Content: evo.Download{URL: srv.URL, Integrity: advSHA512(body)}}
	for i := range 8 {
		group.Task(fmt.Sprintf("d%d", i)).Define(file.Write)
	}
	if err := group.Wait(); err != nil {
		t.Fatalf("concurrent downloads: %v", err)
	}
	_ = out.Finish()
	if got, _ := os.ReadFile(dest); string(got) != string(body) {
		t.Fatalf("destination holds %d bytes, want %d", len(got), len(body))
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("concurrent downloads left %d entries beside the destination", len(entries)-1)
	}
}

// Structural: three Writes to one destination cost one request. The first
// fetches; the two that follow find the destination already matching
// Integrity and make no request.
func TestAdversarial_CurrentDestinationMakesNoRequest(t *testing.T) {
	body := []byte("payload")
	srv, hits := advServer(t, advStatic(body))
	dest := filepath.Join(t.TempDir(), "f")
	for range 3 {
		if err := advDownload(t, 0, dest, srv.URL, advSHA512(body)); err != nil {
			t.Fatal(err)
		}
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("server saw %d requests across three Writes, want 1", n)
	}
}

// Structural: parallel downloads never exceed the Run's concurrency bound.
func TestAdversarial_ParallelDownloadsAreBounded(t *testing.T) {
	const bound, downloads = 4, 24
	body := []byte("payload")
	var mu sync.Mutex
	inflight, peak := 0, 0
	srv, _ := advServer(t, func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		inflight++
		peak = max(peak, inflight)
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		_, _ = w.Write(body)
		mu.Lock()
		inflight--
		mu.Unlock()
	})
	dir := t.TempDir()
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir(), MaxConcurrency: bound})
	defer func() { _ = out.Close() }()
	group := out.Group("downloads")
	for i := range downloads {
		file := evo.File{Path: filepath.Join(dir, fmt.Sprint(i)), Content: evo.Download{URL: srv.URL, Integrity: advSHA512(body)}}
		group.Task(fmt.Sprintf("d%d", i)).Define(file.Write)
	}
	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}
	_ = out.Finish()
	if peak > bound {
		t.Fatalf("peak in-flight downloads = %d, bound %d", peak, bound)
	}
}

// An earlier failed attempt must never leave bytes that a later attempt
// resumes into. The server ignores Range and always answers 200 with the
// whole body, so a client that appended to a partial file would corrupt it.
func TestAdversarial_RetryAfterTruncationNeverResumesIntoCorruption(t *testing.T) {
	body := []byte(strings.Repeat("whole-payload-", 1000))
	var calls atomic.Int64
	srv, _ := advServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			_, _ = w.Write(body[:len(body)/3])
			return
		}
		_, _ = w.Write(body)
	})
	dir := t.TempDir()
	dest := filepath.Join(dir, "pkg.tgz")
	_ = advDownload(t, 0, dest, srv.URL, advSHA512(body))
	if got, err := os.ReadFile(dest); err == nil && string(got) != string(body) {
		t.Fatalf("a failed first attempt published %d of %d bytes", len(got), len(body))
	}
	if err := advDownload(t, 0, dest, srv.URL, advSHA512(body)); err != nil {
		t.Fatalf("retry Write: %v", err)
	}
	if got, _ := os.ReadFile(dest); string(got) != string(body) {
		t.Fatalf("destination holds %d bytes after the retry, want %d", len(got), len(body))
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("retry left %d entries beside the destination", len(entries)-1)
	}
}

// Downloads to different destinations share no lock: both fetches are in
// flight at once. A global or per-directory lock held across the fetch would
// serialize them and the barrier would time out.
func TestAdversarial_DifferentDestinationsFetchConcurrently(t *testing.T) {
	body := []byte("payload")
	var mu sync.Mutex
	inflight, peak := 0, 0
	both := make(chan struct{})
	srv, _ := advServer(t, func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		inflight++
		peak = max(peak, inflight)
		if inflight == 2 {
			close(both)
		}
		mu.Unlock()
		select {
		case <-both:
		case <-time.After(5 * time.Second):
		}
		_, _ = w.Write(body)
	})
	dir := t.TempDir()
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir(), MaxConcurrency: 4})
	defer func() { _ = out.Close() }()
	group := out.Group("downloads")
	for _, name := range []string{"a", "b"} {
		file := evo.File{Path: filepath.Join(dir, name), Content: evo.Download{URL: srv.URL, Integrity: advSHA512(body)}}
		group.Task(name).Define(file.Write)
	}
	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}
	_ = out.Finish()
	if peak != 2 {
		t.Fatalf("peak in-flight fetches = %d, want 2 (downloads to different destinations must overlap)", peak)
	}
}

func BenchmarkDownload_Parallel32x64KiB(b *testing.B) {
	body := make([]byte, 64<<10)
	srv, _ := advServer(b, advStatic(body))
	integrity := advSHA512(body)
	b.SetBytes(32 * int64(len(body)))
	for b.Loop() {
		dir := b.TempDir()
		out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: dir})
		group := out.Group("downloads")
		for i := range 32 {
			file := evo.File{Path: filepath.Join(dir, fmt.Sprint(i)), Content: evo.Download{URL: srv.URL, Integrity: integrity}}
			group.Task(fmt.Sprintf("d%d", i)).Define(file.Write)
		}
		if err := group.Wait(); err != nil {
			b.Fatal(err)
		}
		_ = out.Finish()
		_ = out.Close()
	}
}
