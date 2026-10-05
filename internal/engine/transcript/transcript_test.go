package transcript

import (
	"strings"
	"sync"
	"testing"
)

func TestRetentionBoundByLinesTruncates(t *testing.T) {
	tr := New(Policy{MaxLines: 2})
	tr.Write(Combined, []byte("a\nb\nc\n"))
	if !tr.Truncated() {
		t.Fatal("want Truncated after exceeding MaxLines")
	}
	if got := tr.Text(); got != "[earlier output truncated]\nb\nc" {
		t.Fatalf("Text() = %q", got)
	}
}

func TestRetentionBoundByBytesTruncates(t *testing.T) {
	tr := New(Policy{MaxLines: 1000, MaxBytes: 4})
	tr.Write(Combined, []byte("aaaa\nbbbb\n"))
	if !tr.Truncated() {
		t.Fatal("want Truncated after exceeding MaxBytes")
	}
}

func TestStdoutStderrPendingBuffersNeverMergePartialLine(t *testing.T) {
	tr := New(Policy{})
	tr.Write(Stdout, []byte("out-partial"))
	tr.Write(Stderr, []byte("err-partial"))
	tr.FlushAll()
	if got := tr.StreamText(Stdout); got != "out-partial" {
		t.Fatalf("Stdout StreamText() = %q", got)
	}
	if got := tr.StreamText(Stderr); got != "err-partial" {
		t.Fatalf("Stderr StreamText() = %q", got)
	}
}

func TestDetailCombinedPrefersStderrOnlyWhenStderrHasContent(t *testing.T) {
	tr := New(Policy{})
	tr.Write(Stdout, []byte("out line\n"))
	tr.FlushAll()
	if got := tr.Detail(Combined); got != "out line" {
		t.Fatalf("Detail(Combined) with only stdout = %q, want stdout content", got)
	}

	tr2 := New(Policy{})
	tr2.Write(Stdout, []byte("out line\n"))
	tr2.Write(Stderr, []byte("err line\n"))
	tr2.FlushAll()
	if got := tr2.Detail(Combined); got != "err line" {
		t.Fatalf("Detail(Combined) with both streams = %q, want stderr content", got)
	}
}

func TestDetailPrefixesLastNLinesOnlyForMoreThanOneLine(t *testing.T) {
	tr := New(Policy{})
	tr.Write(Combined, []byte("only\n"))
	tr.FlushAll()
	if got := tr.Detail(Combined); got != "only" {
		t.Fatalf("Detail() single line = %q, want no header", got)
	}

	tr2 := New(Policy{})
	tr2.Write(Combined, []byte("first\nsecond\n"))
	tr2.FlushAll()
	got := tr2.Detail(Combined)
	if !strings.HasPrefix(got, "Last 2 lines:\n") {
		t.Fatalf("Detail() multi line = %q, want \"Last 2 lines:\" header", got)
	}
}

func TestRedactRunsBeforeRetention(t *testing.T) {
	redact := func(s string) string { return strings.ReplaceAll(s, "secret", "REDACTED") }
	tr := New(Policy{Redact: redact})
	tr.Write(Combined, []byte("token=secret\n"))
	tr.FlushAll()
	for _, got := range []string{tr.Text(), tr.StreamText(Combined), tr.Detail(Combined)} {
		if strings.Contains(got, "secret") {
			t.Fatalf("retained/exported text still contains the secret: %q", got)
		}
	}
}

func TestOnLineReceivesRedactedTruncatedLine(t *testing.T) {
	redact := func(s string) string { return strings.ReplaceAll(s, "secret", "REDACTED") }
	var got []string
	var mu sync.Mutex
	tr := New(Policy{Redact: redact, OnLine: func(line string) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, line)
	}})
	long := strings.Repeat("x", maxLineLen+50)
	tr.Write(Combined, []byte("token=secret\n"))
	tr.Write(Combined, []byte(long+"\n"))
	tr.FlushAll()

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("OnLine calls = %d, want 2", len(got))
	}
	if got[0] != "token=REDACTED" {
		t.Fatalf("OnLine first call = %q, want redacted", got[0])
	}
	if strings.Contains(got[1], "x") == false || len([]rune(got[1])) > maxLineLen+1 {
		t.Fatalf("OnLine second call not truncated to maxLineLen: len=%d", len([]rune(got[1])))
	}
}

func TestLineOverDoubleMaxLenWithoutNewlineIsFlushed(t *testing.T) {
	tr := New(Policy{})
	long := strings.Repeat("y", maxLineLen*2+1)
	tr.Write(Combined, []byte(long))
	if tr.Empty() {
		t.Fatal("want a flushed line once pending exceeds 2x maxLineLen, even without a newline")
	}
	if got := tr.StreamText(Combined); got == "" {
		t.Fatal("want the over-long fragment retained")
	}
}
