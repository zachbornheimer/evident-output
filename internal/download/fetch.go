package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// maxRedirects bounds a redirect chain.
	maxRedirects = 10
	// noOverallTimeout is deliberate: a body may legitimately take minutes,
	// so the caller's ctx owns the deadline. Stalls before headers are
	// bounded by headerTimeout.
	noOverallTimeout = 0 * time.Second
	headerTimeout    = 60 * time.Second
)

var (
	clientOnce   sync.Once
	sharedClient *http.Client
)

// client never decodes Content-Encoding, so Integrity covers the bytes as
// served.
func client() *http.Client {
	clientOnce.Do(func() {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DisableCompression = true
		transport.ResponseHeaderTimeout = headerTimeout
		sharedClient = &http.Client{
			Transport: transport,
			Timeout:   noOverallTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return fmt.Errorf("stopped after %d redirects", maxRedirects)
				}
				return checkScheme(req.URL)
			},
		}
	})
	return sharedClient
}

func checkScheme(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme %q is not http or https", u.Scheme)
	}
	return nil
}

// Fetch streams rawURL into w and returns nil only when every byte arrived
// and matches want. On any error the caller must discard what w received.
func Fetch(ctx context.Context, rawURL string, want Expected, w io.Writer) error {
	if rawURL == "" {
		return ErrURLMissing
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: invalid URL: %s", ErrFailed, redact(err.Error(), rawURL))
	}
	shown := u.Redacted()
	if err := checkScheme(u); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrFailed, shown, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("%w: %s: %s", ErrFailed, shown, redact(err.Error(), rawURL))
	}
	resp, err := client().Do(req)
	if err != nil {
		return transportError(ctx, shown, rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%w: %s: %s", ErrFailed, shown, resp.Status)
	}
	h := want.Hasher()
	if _, err := io.Copy(io.MultiWriter(w, h), resp.Body); err != nil {
		return transportError(ctx, shown, rawURL, err)
	}
	if !want.Match(h.Sum(nil)) {
		return fmt.Errorf("%w: %s: bytes do not match the %s integrity", ErrIntegrity, shown, want.Algorithm())
	}
	return nil
}

// transportError keeps context errors matchable and strips URL credentials.
func transportError(ctx context.Context, shown, raw string, err error) error {
	if cause := ctx.Err(); cause != nil {
		return fmt.Errorf("download %s: %w", shown, cause)
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	return fmt.Errorf("%w: %s: %s", ErrFailed, shown, redact(err.Error(), raw))
}

// redact removes the userinfo secret of raw from msg.
func redact(msg, raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return msg
	}
	if pw, ok := u.User.Password(); ok && pw != "" {
		msg = strings.ReplaceAll(msg, pw, "xxxxx")
	}
	return strings.ReplaceAll(msg, u.User.String()+"@", "")
}
