package patch

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func mustParse(t *testing.T, diff string) []File {
	t.Helper()
	files, err := Parse([]byte(diff))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return files
}

func mustApply(t *testing.T, f File, old string) string {
	t.Helper()
	got, err := f.Apply([]byte(old))
	if err != nil {
		t.Fatalf("Apply %s: %v", f.Path, err)
	}
	return string(got)
}

const gitModify = `diff --git a/greeting.txt b/greeting.txt
index 3b18e51..a042389 100644
--- a/greeting.txt
+++ b/greeting.txt
@@ -1,3 +1,3 @@
 hello
-world
+there
 bye
`

func TestParseGitModification(t *testing.T) {
	files := mustParse(t, gitModify)
	if len(files) != 1 {
		t.Fatalf("got %d files", len(files))
	}
	f := files[0]
	if f.Path != "greeting.txt" || f.Create || f.Mode != 0 {
		t.Fatalf("got %+v", f)
	}
	if got := mustApply(t, f, "hello\nworld\nbye\n"); got != "hello\nthere\nbye\n" {
		t.Fatalf("Apply = %q", got)
	}
}

func TestParsePlainUnifiedDiffWithTimestamps(t *testing.T) {
	diff := "--- notes.txt\t2026-09-23 10:00:00\n+++ notes.txt\t2026-09-23 10:01:00\n@@ -2 +2,2 @@\n-b\n+B\n+c\n"
	f := mustParse(t, diff)[0]
	if f.Path != "notes.txt" {
		t.Fatalf("Path = %q", f.Path)
	}
	if got := mustApply(t, f, "a\nb\n"); got != "a\nB\nc\n" {
		t.Fatalf("Apply = %q", got)
	}
}

func TestParseMultiFileAndMultiHunk(t *testing.T) {
	diff := gitModify + `diff --git a/list.txt b/list.txt
--- a/list.txt
+++ b/list.txt
@@ -1,2 +1,2 @@
-1
+one
 2
@@ -5,2 +5,2 @@
 5
-6
+six
`
	files := mustParse(t, diff)
	if len(files) != 2 || files[0].Path != "greeting.txt" || files[1].Path != "list.txt" {
		t.Fatalf("got %+v", files)
	}
	if got := mustApply(t, files[1], "1\n2\n3\n4\n5\n6\n"); got != "one\n2\n3\n4\n5\nsix\n" {
		t.Fatalf("Apply = %q", got)
	}
}

func TestParseCreation(t *testing.T) {
	diff := `diff --git a/bin/run b/bin/run
new file mode 100755
index 0000000..e69de29
--- /dev/null
+++ b/bin/run
@@ -0,0 +1,2 @@
+#!/bin/sh
+echo hi
`
	f := mustParse(t, diff)[0]
	if f.Path != "bin/run" || !f.Create || f.Mode != 0o755 {
		t.Fatalf("got %+v", f)
	}
	if got := mustApply(t, f, ""); got != "#!/bin/sh\necho hi\n" {
		t.Fatalf("Apply = %q", got)
	}
}

func TestParseEmptyFileCreation(t *testing.T) {
	diff := "diff --git a/empty b/empty\nnew file mode 100644\nindex 0000000..e69de29\n"
	f := mustParse(t, diff)[0]
	if f.Path != "empty" || !f.Create || f.Mode != 0o644 || len(f.Hunks) != 0 {
		t.Fatalf("got %+v", f)
	}
	if got := mustApply(t, f, ""); got != "" {
		t.Fatalf("Apply = %q", got)
	}
}

func TestParseModeChange(t *testing.T) {
	diff := "diff --git a/tool.sh b/tool.sh\nold mode 100644\nnew mode 100755\n"
	f := mustParse(t, diff)[0]
	if f.Path != "tool.sh" || f.Create || f.Mode != fs.FileMode(0o755) {
		t.Fatalf("got %+v", f)
	}
	if got := mustApply(t, f, "x\n"); got != "x\n" {
		t.Fatalf("Apply = %q", got)
	}
}

func TestApplyNoNewlineAtEndOfFile(t *testing.T) {
	diff := "--- a/f\n+++ b/f\n@@ -1 +1 @@\n-old\n\\ No newline at end of file\n+new\n\\ No newline at end of file\n"
	f := mustParse(t, diff)[0]
	if got := mustApply(t, f, "old"); got != "new" {
		t.Fatalf("Apply = %q", got)
	}
	if _, err := f.Apply([]byte("old\n")); !errors.Is(err, ErrDoesNotApply) {
		t.Fatalf("a trailing newline the patch says is absent must not match: %v", err)
	}
}

func TestApplyContextMismatchFails(t *testing.T) {
	f := mustParse(t, gitModify)[0]
	_, err := f.Apply([]byte("hello\nplanet\nbye\n"))
	if !errors.Is(err, ErrDoesNotApply) {
		t.Fatalf("err = %v, want ErrDoesNotApply", err)
	}
}

func TestUnsupportedFormsFailExplicitly(t *testing.T) {
	cases := map[string]struct {
		diff string
		want error
	}{
		"git deletion": {
			"diff --git a/gone b/gone\ndeleted file mode 100644\nindex e69de29..0000000\n",
			ErrDeleteUnsupported,
		},
		"plain deletion": {
			"--- a/gone\n+++ /dev/null\n@@ -1 +0,0 @@\n-x\n",
			ErrDeleteUnsupported,
		},
		"git rename": {
			"diff --git a/old b/new\nsimilarity index 100%\nrename from old\nrename to new\n",
			ErrRenameUnsupported,
		},
		"git copy": {
			"diff --git a/old b/new\nsimilarity index 100%\ncopy from old\ncopy to new\n",
			ErrRenameUnsupported,
		},
		"plain path change": {
			"--- a/old\n+++ b/new\n@@ -1 +1 @@\n-x\n+y\n",
			ErrRenameUnsupported,
		},
		"binary differ": {
			"diff --git a/img.png b/img.png\nindex 1..2 100644\nBinary files a/img.png and b/img.png differ\n",
			ErrBinaryUnsupported,
		},
		"git binary patch": {
			"diff --git a/img.png b/img.png\nindex 1..2 100644\nGIT binary patch\nliteral 3\nKcmZ?wbN~PV\n\nliteral 0\nHcmV?d00001\n\n",
			ErrBinaryUnsupported,
		},
		"symlink creation": {
			"diff --git a/link b/link\nnew file mode 120000\n--- /dev/null\n+++ b/link\n@@ -0,0 +1 @@\n+target\n\\ No newline at end of file\n",
			ErrUnsupported,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(tc.diff))
			if !errors.Is(err, tc.want) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("err = %v, want %v (an ErrUnsupported)", err, tc.want)
			}
		})
	}
}

func TestMalformedDiffsFail(t *testing.T) {
	cases := map[string]string{
		"empty":              "",
		"no file headers":    "just some prose\n",
		"truncated hunk":     "--- a/f\n+++ b/f\n@@ -1,2 +1,2 @@\n a\n",
		"bad hunk line":      "--- a/f\n+++ b/f\n@@ -1 +1 @@\n*a\n",
		"bad hunk header":    "--- a/f\n+++ b/f\n@@ -x +1 @@\n a\n",
		"duplicate path":     "--- a/f\n+++ b/f\n@@ -1 +1 @@\n-a\n+b\n--- a/f\n+++ b/f\n@@ -1 +1 @@\n-b\n+c\n",
		"absolute path":      "--- /etc/passwd\n+++ /etc/passwd\n@@ -1 +1 @@\n-a\n+b\n",
		"escaping path":      "--- a/../outside\n+++ b/../outside\n@@ -1 +1 @@\n-a\n+b\n",
		"hunks out of order": "--- a/f\n+++ b/f\n@@ -3 +3 @@\n-c\n+C\n@@ -1 +1 @@\n-a\n+A\n",
	}
	for name, diff := range cases {
		t.Run(name, func(t *testing.T) {
			files, err := Parse([]byte(diff))
			if err == nil && len(files) == 1 {
				_, err = files[0].Apply([]byte("a\nb\nc\n"))
			}
			if !errors.Is(err, ErrMalformed) {
				t.Fatalf("err = %v, want ErrMalformed", err)
			}
		})
	}
}

func TestParseIgnoresPreambleAndCRLFContent(t *testing.T) {
	diff := "From abc Mon Sep 17 00:00:00 2001\nSubject: change\n\n---\n" +
		"diff --git a/w.txt b/w.txt\n--- a/w.txt\n+++ b/w.txt\n@@ -1 +1 @@\n-a\r\n+b\r\n"
	f := mustParse(t, diff)[0]
	if got := mustApply(t, f, "a\r\n"); got != "b\r\n" {
		t.Fatalf("Apply = %q", got)
	}
	if !strings.Contains(f.Path, "w.txt") {
		t.Fatalf("Path = %q", f.Path)
	}
}
