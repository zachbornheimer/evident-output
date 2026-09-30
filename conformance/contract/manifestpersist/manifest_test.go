// Package manifestpersist_test binds contract §22 and §30 "Manifest
// persistence" rules to the public evo API.
package manifestpersist_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/manifest"
)

const (
	saveFailureMarker = "manifest not saved"
	readOnlyDir       = 0o500
	writableDir       = 0o700
	writerTaskCount   = 25
)

// syncBuffer is a write-safe capture of the human stream.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func newOutput(state string, stream *syncBuffer) *evo.Output {
	return evo.Init(evo.Config{
		Isolated: true, Plain: true, Color: evo.ColorNever,
		StateDir: state, Stdout: stream, Stderr: stream,
	})
}

func manifestPath(t *testing.T, state string) string {
	t.Helper()
	path, err := manifest.Locate(manifest.Config{StateDir: state}, manifest.NewOSEnvironment())
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func readDocument(t *testing.T, path string) manifest.Document {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("manifest not on disk: %v", err)
	}
	var doc manifest.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("manifest is not a complete document: %v", err)
	}
	return doc
}

func writeFileTask(out *evo.Output, name, path string) *evo.TaskHandle {
	task := out.Task(name)
	task.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte(name)})
	})
	return task
}

func TestC22_019_MissingManifestIsACleanFirstRun(t *testing.T) {
	state := t.TempDir()
	path := manifestPath(t, state)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("precondition: manifest already exists (%v)", err)
	}
	stream := &syncBuffer{}
	out := newOutput(state, stream)
	if err := writeFileTask(out, "create", filepath.Join(t.TempDir(), "f.txt")).Wait(); err != nil {
		t.Fatalf("first run without a manifest: %v", err)
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if out.Conclusion().Warned || strings.Contains(stream.String(), "manifest") {
		t.Fatalf("a missing manifest is not worth a warning:\n%s", stream.String())
	}
	if doc := readDocument(t, path); len(doc.Tasks) == 0 {
		t.Fatal("the first run did not create a manifest with its record")
	}
}

func TestC22_019_ManifestWritesAreAtomicForConcurrentReaders(t *testing.T) {
	state := t.TempDir()
	path := manifestPath(t, state)
	out := newOutput(state, &syncBuffer{})
	t.Cleanup(func() { _ = out.Close() })

	stop, torn := make(chan struct{}), make(chan error, 1)
	var readers sync.WaitGroup
	readers.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var doc manifest.Document
			if err := json.Unmarshal(raw, &doc); err != nil {
				select {
				case torn <- err:
				default:
				}
				return
			}
		}
	})

	dir := t.TempDir()
	tasks := make([]*evo.TaskHandle, writerTaskCount)
	for i := range tasks {
		name := "write " + string(rune('a'+i))
		tasks[i] = writeFileTask(out, name, filepath.Join(dir, name))
	}
	for _, task := range tasks {
		if err := task.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	_ = out.Finish()
	close(stop)
	readers.Wait()

	select {
	case err := <-torn:
		t.Fatalf("a reader observed a partially written manifest: %v", err)
	default:
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") || strings.Contains(entry.Name(), ".tmp-") {
			t.Errorf("a temporary write file was left behind: %s", entry.Name())
		}
	}
	if doc := readDocument(t, path); len(doc.Tasks) != writerTaskCount {
		t.Fatalf("manifest holds %d task records, want %d", len(doc.Tasks), writerTaskCount)
	}
}

func TestC30_085_AFailedManifestWriteIsRetriedOnTheNextFlush(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Fatal("directory permissions do not bind root; run the contract tests unprivileged")
	}
	state := t.TempDir()
	t.Cleanup(func() { _ = os.Chmod(state, writableDir) })
	stream := &syncBuffer{}
	out := newOutput(state, stream)

	task := out.Task("file")
	task.Define(func(ctx context.Context) error {
		if err := evo.File(ctx, evo.FileSpec{Path: filepath.Join(t.TempDir(), "f.txt"), Contents: []byte("x")}); err != nil {
			return err
		}
		return os.Chmod(state, readOnlyDir)
	})
	if err := task.Wait(); err != nil {
		t.Fatalf("file task: %v", err)
	}
	_ = out.Finish()
	if !strings.Contains(stream.String(), saveFailureMarker) {
		t.Fatalf("the failed save was not reported:\n%s", stream.String())
	}

	if err := os.Chmod(state, writableDir); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("Close after the directory became writable = %v, want the retried write to succeed", err)
	}
	if doc := readDocument(t, manifestPath(t, state)); len(doc.Tasks) != 1 {
		t.Fatalf("manifest holds %d task records after the retry, want 1: %v", len(doc.Tasks), doc.Tasks)
	}
}
