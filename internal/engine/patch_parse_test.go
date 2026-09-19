package engine

import (
	"errors"
	"testing"
)

func TestPatchParse_RejectsDelete(t *testing.T) {
	t.Parallel()
	diff := []byte("" +
		"--- a/gone.txt\n" +
		"+++ /dev/null\n" +
		"@@ -1 +0,0 @@\n" +
		"-bye\n")
	files, err := parseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files=%d", len(files))
	}
	if err := files[0].unsupported(); !errors.Is(err, ErrPatchDelete) {
		t.Fatalf("unsupported=%v, want ErrPatchDelete", err)
	}
}

func TestPatchParse_RejectsRenameHeaders(t *testing.T) {
	t.Parallel()
	diff := []byte("" +
		"diff --git a/old.txt b/new.txt\n" +
		"rename from old.txt\n" +
		"rename to new.txt\n" +
		"--- a/old.txt\n" +
		"+++ b/new.txt\n" +
		"@@ -1 +1 @@\n" +
		"-a\n" +
		"+b\n")
	files, err := parseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := files[0].unsupported(); !errors.Is(err, ErrPatchRename) {
		t.Fatalf("unsupported=%v, want ErrPatchRename", err)
	}
}

func TestPatchParse_RejectsBinary(t *testing.T) {
	t.Parallel()
	diff := []byte("" +
		"diff --git a/foo.bin b/foo.bin\n" +
		"GIT binary patch\n" +
		"literal 4\n")
	if _, err := parseUnifiedDiff(diff); !errors.Is(err, ErrPatchBinary) {
		t.Fatalf("parse=%v, want ErrPatchBinary", err)
	}
}

func TestPatchApply_OneFileReplace(t *testing.T) {
	t.Parallel()
	diff := []byte("" +
		"--- a/hello.txt\n" +
		"+++ b/hello.txt\n" +
		"@@ -1,2 +1,2 @@\n" +
		" hello\n" +
		"-world\n" +
		"+there\n")
	files, err := parseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, err := applyHunks([]byte("hello\nworld\n"), files[0].hunks)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if string(got) != "hello\nthere\n" {
		t.Fatalf("got %q", got)
	}
}

func TestPatchApply_NewFile(t *testing.T) {
	t.Parallel()
	diff := []byte("" +
		"diff --git a/new.txt b/new.txt\n" +
		"new file mode 100644\n" +
		"--- /dev/null\n" +
		"+++ b/new.txt\n" +
		"@@ -0,0 +1,2 @@\n" +
		"+alpha\n" +
		"+beta\n")
	files, err := parseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := files[0].unsupported(); err != nil {
		t.Fatalf("unsupported: %v", err)
	}
	got, err := applyHunks(nil, files[0].hunks)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if string(got) != "alpha\nbeta\n" {
		t.Fatalf("got %q", got)
	}
	if files[0].mode.Perm() != 0o644 {
		t.Fatalf("mode=%#o", files[0].mode.Perm())
	}
}

func TestPatchApply_MalformedHunk(t *testing.T) {
	t.Parallel()
	diff := []byte("" +
		"--- a/hello.txt\n" +
		"+++ b/hello.txt\n" +
		"@@ -1,2 +1,2 @@\n" +
		" hello\n" +
		"-nope\n" +
		"+there\n")
	files, err := parseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := applyHunks([]byte("hello\nworld\n"), files[0].hunks); !errors.Is(err, ErrPatchMalformed) {
		t.Fatalf("apply=%v, want ErrPatchMalformed", err)
	}
}
