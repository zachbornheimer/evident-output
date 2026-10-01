package engine

import "errors"

// ZYS-1382 filesystem vocabulary errors: File, Tree, their content
// producers (Bytes, Download, Extract), Find, Exec, and Task Basis.
var (
	// ErrPathMissing is a File or Tree whose Path is empty.
	ErrPathMissing = errors.New("evo: Path is required")
	// ErrContentMissing is a Write whose Content is nil: no desired content
	// was declared, so there is nothing to establish.
	ErrContentMissing = errors.New("evo: Content is required to Write")
	// ErrVerifyMismatch is a File or Tree whose observed state differs from
	// its declared Content.
	ErrVerifyMismatch = errors.New("evo: observed state does not match declared Content")
	// ErrTreePathTypeMismatch is a Tree whose Path holds a non-directory.
	ErrTreePathTypeMismatch = errors.New("evo: Tree path is not a directory")
	// ErrIntegrityMismatch is a Download whose bytes do not match its
	// Integrity, or whose Integrity is empty or unparseable.
	ErrIntegrityMismatch = errors.New("evo: Download integrity mismatch")
	// ErrDownloadFailed is a Download whose server did not answer 2xx.
	ErrDownloadFailed = errors.New("evo: Download failed")
	// ErrDownloadURLMissing is a Download whose URL is empty.
	ErrDownloadURLMissing = errors.New("evo: Download URL is required")
	// ErrExtractMalformed is an Extract archive that is empty or unreadable.
	ErrExtractMalformed = errors.New("evo: Extract archive is malformed")
	// ErrExtractUnsafeEntry is an archive entry that would escape the
	// destination or is not a regular file, directory, or contained link.
	ErrExtractUnsafeEntry = errors.New("evo: Extract archive has an unsafe entry")
	// ErrFindNamesMissing is a Find called with no names.
	ErrFindNamesMissing = errors.New("evo: Find needs at least one name")
	// ErrExecPathMissing is an Exec whose Path is empty.
	ErrExecPathMissing = errors.New("evo: Exec.Path is required")
	// ErrBasisAfterDefine is recorded when TaskHandle.Basis is called after
	// Define; the call is ignored.
	ErrBasisAfterDefine = errors.New("evo: Basis called after Define")
	// ErrNotImplemented is the ZYS-1382 skeleton's answer from a primitive
	// whose behavior has not landed yet.
	ErrNotImplemented = errors.New("evo: not implemented yet (ZYS-1382)")
)
