package download

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sriOf(b []byte) string {
	s := sha512.Sum512(b)
	return "sha512-" + base64.StdEncoding.EncodeToString(s[:])
}

func TestFetchVerifiesAndStreams(t *testing.T) {
	body := []byte("hello")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	defer srv.Close()
	src, err := NewSource(srv.URL, sriOf(body))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := src.Fill(context.Background(), &buf); err != nil || buf.String() != "hello" {
		t.Fatalf("Fill = %v, %q", err, buf.String())
	}
	bad, _ := NewSource(srv.URL, sriOf([]byte("x")))
	if err := bad.Fill(context.Background(), &bytes.Buffer{}); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("mismatch = %v", err)
	}
}

func TestFetchRejectsNonHTTPAndRedactsCredentials(t *testing.T) {
	src, _ := NewSource("file:///etc/passwd", sriOf(nil))
	if err := src.Fill(context.Background(), &bytes.Buffer{}); !errors.Is(err, ErrFailed) {
		t.Fatalf("file scheme = %v", err)
	}
	src, _ = NewSource("http://u:s3kr3t@127.0.0.1:1/x", sriOf(nil))
	err := src.Fill(context.Background(), &bytes.Buffer{})
	if err == nil || strings.Contains(err.Error(), "s3kr3t") {
		t.Fatalf("error leaks secret: %v", err)
	}
}

func TestParseRejectsBadIntegrity(t *testing.T) {
	for _, s := range []string{"", " ", "sha512-", "md5-AAAA", "zz", strings.Repeat("0", 63)} {
		if _, err := Parse(s); !errors.Is(err, ErrIntegrity) {
			t.Errorf("Parse(%q) = %v", s, err)
		}
	}
}
