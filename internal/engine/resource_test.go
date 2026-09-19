package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResourceGraphReadReadOverlap(t *testing.T) {
	t.Parallel()
	g := newResourceGraph()
	ctx := context.Background()
	drop1, err := g.acquire(ctx, nil, []resourceHold{{path: "a", kind: holdRead}})
	if err != nil {
		t.Fatal(err)
	}
	drop2, err := g.acquire(ctx, nil, []resourceHold{{path: "a", kind: holdRead}})
	if err != nil {
		t.Fatal(err)
	}
	drop1()
	drop2()
}

func TestResourceGraphWriteWaitsForRead(t *testing.T) {
	t.Parallel()
	g := newResourceGraph()
	ctx := context.Background()
	dropRead, err := g.acquire(ctx, nil, []resourceHold{{path: "a", kind: holdRead}})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		drop, err := g.acquire(ctx, nil, []resourceHold{{path: "a", kind: holdWrite}})
		if err == nil {
			drop()
		}
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("write acquired while read held: %v", err)
	case <-time.After(40 * time.Millisecond):
	}
	dropRead()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("write did not acquire after read released")
	}
}

func TestResourceGraphCancelReleasesWaiter(t *testing.T) {
	t.Parallel()
	g := newResourceGraph()
	dropWrite, err := g.acquire(context.Background(), nil, []resourceHold{{path: "a", kind: holdWrite}})
	if err != nil {
		t.Fatal(err)
	}
	defer dropWrite()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := g.acquire(ctx, nil, []resourceHold{{path: "a", kind: holdRead}})
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled waiter returned nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled waiter did not return")
	}
}

func TestCanonicalFilePath_MissingUsesParent(t *testing.T) {
	t.Parallel()
	realDir := t.TempDir()
	linkDir := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatal(err)
	}
	viaLink := filepath.Join(linkDir, "new.txt")
	viaReal := filepath.Join(realDir, "new.txt")
	gotLink, err := canonicalFilePath(viaLink)
	if err != nil {
		t.Fatal(err)
	}
	gotReal, err := canonicalFilePath(viaReal)
	if err != nil {
		t.Fatal(err)
	}
	if gotLink != gotReal {
		t.Fatalf("missing-file identity: link=%q real=%q", gotLink, gotReal)
	}
}

func TestCanonicalFilePath_SymlinkAlias(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(real, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	gotReal, err := canonicalFilePath(real)
	if err != nil {
		t.Fatal(err)
	}
	gotLink, err := canonicalFilePath(link)
	if err != nil {
		t.Fatal(err)
	}
	if gotReal != gotLink {
		t.Fatalf("symlink identity: real=%q link=%q", gotReal, gotLink)
	}
}
