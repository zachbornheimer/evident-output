package evo_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"

	evo "github.com/zachbornheimer/evident-output"
)

// runExampleTask runs fn as the Define callback of one Task on an isolated,
// silent Output whose state lives in stateDir: the ctx File and Tree
// writes need. It returns the Task's error.
func runExampleTask(stateDir string, fn func(ctx context.Context) error) error {
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: stateDir,
	})
	err := out.Task("example").Define(fn).Wait()
	if finishErr := out.Finish(); err == nil {
		err = finishErr
	}
	return err
}

// exampleDir makes a scratch directory; go/doc Examples have no t.TempDir.
func exampleDir() (string, func()) {
	dir, err := os.MkdirTemp("", "evo-example-tree")
	if err != nil {
		panic(err)
	}
	return dir, func() { _ = os.RemoveAll(dir) }
}

// plantExampleTree writes files (relative path -> content) under root.
func plantExampleTree(root string, files map[string]string) {
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			panic(err)
		}
	}
}

// ExampleBytes declares literal content for a File; Write establishes it.
func ExampleBytes() {
	dir, cleanup := exampleDir()
	defer cleanup()
	config := evo.File{Path: filepath.Join(dir, "config.toml"), Content: evo.Bytes("debug = true\n")}

	err := runExampleTask(dir, config.Write)
	got, _ := config.Read(context.Background())
	fmt.Printf("%v %q\n", err, got)
	// Output:
	// <nil> "debug = true\n"
}

// ExampleFileContent is what a File should hold: Bytes or Download.
func ExampleFileContent() {
	contents := []evo.FileContent{
		evo.Bytes("literal"),
		evo.Download{URL: "https://example.com/tool.tgz", Integrity: "sha256-..."},
	}
	for _, c := range contents {
		if d, ok := c.(evo.Download); ok {
			fmt.Println("download", d.URL)
			continue
		}
		fmt.Println("literal bytes")
	}
	// Output:
	// literal bytes
	// download https://example.com/tool.tgz
}

// ExampleDownload fetches File content from a URL and verifies it against
// Integrity before it reaches Path. It needs the network, so it is
// compiled but not run.
func ExampleDownload() {
	tool := evo.File{
		Path: "bin/tool.tgz",
		Content: evo.Download{
			URL:       "https://example.com/tool-1.2.0.tgz",
			Integrity: "sha512-<base64 digest from the lockfile>",
		},
	}
	_ = runExampleTask(".", tool.Write)
}

// ExampleFind returns every regular file under a root with one of the
// given base names, sorted by path.
func ExampleFind() {
	dir, cleanup := exampleDir()
	defer cleanup()
	plantExampleTree(dir, map[string]string{
		"a/go.mod": "module a\n", "b/c/go.mod": "module c\n", "b/main.go": "package main\n",
	})

	files, err := evo.Find(context.Background(), dir, "go.mod")
	for _, f := range files {
		rel, _ := filepath.Rel(dir, f.Path)
		fmt.Println(filepath.ToSlash(rel))
	}
	fmt.Println(err)
	// Output:
	// a/go.mod
	// b/c/go.mod
	// <nil>
}

// ExampleTree reads a directory as one value and compares it with another.
func ExampleTree() {
	dir, cleanup := exampleDir()
	defer cleanup()
	plantExampleTree(dir, map[string]string{"a/x.txt": "x", "b/x.txt": "x"})
	a, b := evo.Tree{Path: filepath.Join(dir, "a")}, evo.Tree{Path: filepath.Join(dir, "b")}

	same, err := a.Equal(context.Background(), b)
	files, _ := a.Read(context.Background())
	fmt.Println(same, err, len(files))
	// Output:
	// true <nil> 1
}

// ExampleTreeContent is what a Tree should hold: Extract or Clone.
func ExampleTreeContent() {
	contents := []evo.TreeContent{
		evo.Extract{File: evo.File{Path: "pkg.tgz"}, Root: "package"},
		evo.Clone{From: evo.Tree{Path: "store/pkg"}},
	}
	for _, c := range contents {
		switch c := c.(type) {
		case evo.Extract:
			fmt.Println("extract", c.File.Path)
		case evo.Clone:
			fmt.Println("clone", c.From.Path)
		}
	}
	// Output:
	// extract pkg.tgz
	// clone store/pkg
}

// ExampleClone copies one Tree into another; the copy digests the same.
func ExampleClone() {
	dir, cleanup := exampleDir()
	defer cleanup()
	plantExampleTree(dir, map[string]string{"store/pkg/index.js": "module.exports = 1\n"})
	from := evo.Tree{Path: filepath.Join(dir, "store/pkg")}
	to := evo.Tree{Path: filepath.Join(dir, "node_modules/pkg"), Content: evo.Clone{From: from, Writable: true}}

	err := runExampleTask(dir, to.Write)
	same, _ := to.Equal(context.Background(), from)
	fmt.Println(err, same)
	// Output:
	// <nil> true
}

// ExampleExtract unpacks an archive into a Tree, stripping a leading Root.
func ExampleExtract() {
	dir, cleanup := exampleDir()
	defer cleanup()
	archive := filepath.Join(dir, "pkg.tgz")
	if err := os.WriteFile(archive, exampleTarGz(map[string]string{"package/index.js": "x\n"}), 0o644); err != nil {
		panic(err)
	}
	pkg := evo.Tree{
		Path:    filepath.Join(dir, "node_modules/pkg"),
		Content: evo.Extract{File: evo.File{Path: archive}, Root: "package"},
	}

	err := runExampleTask(dir, pkg.Write)
	got, _ := evo.File{Path: filepath.Join(pkg.Path, "index.js")}.Read(context.Background())
	fmt.Printf("%v %q\n", err, got)
	// Output:
	// <nil> "x\n"
}

// exampleTarGz builds a gzipped tar of regular files.
func exampleTarGz(files map[string]string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			panic(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			panic(err)
		}
	}
	if err := tw.Close(); err != nil {
		panic(err)
	}
	if err := gz.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// ExampleExclude leaves matching entries out of a Tree comparison.
func ExampleExclude() {
	dir, cleanup := exampleDir()
	defer cleanup()
	plantExampleTree(dir, map[string]string{"a/main.go": "x", "a/.git/HEAD": "1", "b/main.go": "x", "b/.git/HEAD": "2"})
	a, b := evo.Tree{Path: filepath.Join(dir, "a")}, evo.Tree{Path: filepath.Join(dir, "b")}

	withGit, _ := a.Equal(context.Background(), b)
	withoutGit, _ := a.Equal(context.Background(), b, evo.Exclude(`.*/\.git/.*`))
	fmt.Println(withGit, withoutGit)
	// Output:
	// false true
}

// ExampleChecksumOption adjusts a Tree checksum; Exclude is the one kind.
func ExampleChecksumOption() {
	dir, cleanup := exampleDir()
	defer cleanup()
	plantExampleTree(dir, map[string]string{"main.go": "x", "build/out.bin": "generated"})
	opts := []evo.ChecksumOption{evo.Exclude(`^/build/`)}

	full, _ := evo.Tree{Path: dir}.Checksum(context.Background())
	source, _ := evo.Tree{Path: dir}.Checksum(context.Background(), opts...)
	fmt.Println(full != source)
	// Output:
	// true
}

// ExampleTree_ReplaceTree swaps new content in at Path only if Path still
// holds the tree the caller observed.
func ExampleTree_ReplaceTree() {
	dir, cleanup := exampleDir()
	defer cleanup()
	plantExampleTree(dir, map[string]string{"next/index.js": "2", "pkg/index.js": "1"})
	pkg := evo.Tree{Path: filepath.Join(dir, "pkg"), Content: evo.Clone{From: evo.Tree{Path: filepath.Join(dir, "next")}}}
	observed, _ := evo.Tree{Path: pkg.Path}.Checksum(context.Background())

	var res evo.ReplaceResult
	err := runExampleTask(dir, func(ctx context.Context) (err error) {
		res, err = pkg.ReplaceTree(ctx, observed)
		return err
	})
	fmt.Println(err, res.Published)
	// Output:
	// <nil> true
}

// ExampleReplaceResult reports whether ReplaceTree published: a Path that
// already holds Content is satisfied and left alone.
func ExampleReplaceResult() {
	dir, cleanup := exampleDir()
	defer cleanup()
	plantExampleTree(dir, map[string]string{"next/index.js": "2", "pkg/index.js": "2"})
	pkg := evo.Tree{Path: filepath.Join(dir, "pkg"), Content: evo.Clone{From: evo.Tree{Path: filepath.Join(dir, "next")}}}
	observed, _ := evo.Tree{Path: pkg.Path}.Checksum(context.Background())

	var res evo.ReplaceResult
	err := runExampleTask(dir, func(ctx context.Context) (err error) {
		res, err = pkg.ReplaceTree(ctx, observed)
		return err
	})
	fmt.Println(err, res.Published)
	// Output:
	// <nil> false
}

// ExampleRepublish swaps Content in even when Path already holds it, for a
// caller that needs a fresh copy (say, to drop shared file data).
func ExampleRepublish() {
	dir, cleanup := exampleDir()
	defer cleanup()
	plantExampleTree(dir, map[string]string{"next/index.js": "2", "pkg/index.js": "2"})
	pkg := evo.Tree{Path: filepath.Join(dir, "pkg"), Content: evo.Clone{From: evo.Tree{Path: filepath.Join(dir, "next")}}}
	observed, _ := evo.Tree{Path: pkg.Path}.Checksum(context.Background())

	var res evo.ReplaceResult
	err := runExampleTask(dir, func(ctx context.Context) (err error) {
		res, err = pkg.ReplaceTree(ctx, observed, evo.Republish())
		return err
	})
	fmt.Println(err, res.Published)
	// Output:
	// <nil> true
}

// ExampleReplaceOption adjusts a ReplaceTree; Republish is the one kind.
func ExampleReplaceOption() {
	opts := []evo.ReplaceOption{evo.Republish()}
	fmt.Println(len(opts))
	// Output:
	// 1
}

// ExampleTree_Recover settles Path after an interrupted Replace, by digest
// alone. Here the Replace never started, so Path is intact.
func ExampleTree_Recover() {
	dir, cleanup := exampleDir()
	defer cleanup()
	plantExampleTree(dir, map[string]string{"next/index.js": "2", "pkg/index.js": "1"})
	pkg := evo.Tree{Path: filepath.Join(dir, "pkg"), Content: evo.Clone{From: evo.Tree{Path: filepath.Join(dir, "next")}}}
	observed, _ := evo.Tree{Path: pkg.Path}.Checksum(context.Background())

	var res evo.RecoverResult
	err := runExampleTask(dir, func(ctx context.Context) (err error) {
		res, err = pkg.Recover(ctx, observed)
		return err
	})
	fmt.Println(err, res.State == evo.RecoverIntact, len(res.Leftovers))
	// Output:
	// <nil> true 0
}

// ExampleRecoverResult carries the settled state and any leftover trees
// Recover kept because it could not prove them redundant.
func ExampleRecoverResult() {
	res := evo.RecoverResult{State: evo.RecoverUnrecoverable, Leftovers: []string{".pkg.evo-old"}}
	if res.State == evo.RecoverUnrecoverable {
		fmt.Println("inspect by hand:", res.Leftovers)
	}
	// Output:
	// inspect by hand: [.pkg.evo-old]
}

// ExampleRecoverState names how Recover left Path.
func ExampleRecoverState() {
	for _, s := range []evo.RecoverState{
		evo.RecoverIntact, evo.RecoverCompletedReplacement, evo.RecoverRestoredOriginal, evo.RecoverUnrecoverable,
	} {
		switch s {
		case evo.RecoverIntact:
			fmt.Println("intact: Path holds the original")
		case evo.RecoverCompletedReplacement:
			fmt.Println("completed: Path holds the replacement")
		case evo.RecoverRestoredOriginal:
			fmt.Println("restored: the original is back at Path")
		case evo.RecoverUnrecoverable:
			fmt.Println("unrecoverable: nothing changed")
		}
	}
	// Output:
	// intact: Path holds the original
	// completed: Path holds the replacement
	// restored: the original is back at Path
	// unrecoverable: nothing changed
}

// ExampleTaskHandle_Basis makes a Task current while its inputs are
// unchanged: the second run with the same StateDir skips Define.
func ExampleTaskHandle_Basis() {
	dir, cleanup := exampleDir()
	defer cleanup()
	plantExampleTree(dir, map[string]string{"package.json": "{}"})
	var runs atomic.Int32
	install := func() {
		out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: dir})
		_ = out.Task("install").
			Basis(evo.File{Path: filepath.Join(dir, "package.json")}).
			Define(func(context.Context) error { runs.Add(1); return nil }).
			Wait()
		_ = out.Finish()
	}

	install()
	install()
	fmt.Println(runs.Load())
	// Output:
	// 1
}
