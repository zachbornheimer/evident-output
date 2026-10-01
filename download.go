package evo

import "github.com/zachbornheimer/evident-output/internal/engine"

// Download is file content fetched from URL and verified against Integrity
// before it reaches the destination.
type Download struct {
	URL string
	// Integrity is an SRI string ("sha512-<b64>", "sha384-", "sha256-") or
	// plain hex sha256 / sha1.
	Integrity string
}

func (Download) fileContent() {}

// Download errors.
var (
	// ErrIntegrityMismatch is downloaded bytes that do not match Integrity,
	// or an empty or unparseable Integrity.
	ErrIntegrityMismatch = engine.ErrIntegrityMismatch
	// ErrDownloadFailed is a non-2xx response.
	ErrDownloadFailed = engine.ErrDownloadFailed
	// ErrDownloadURLMissing is a Download whose URL is empty.
	ErrDownloadURLMissing = engine.ErrDownloadURLMissing
)
