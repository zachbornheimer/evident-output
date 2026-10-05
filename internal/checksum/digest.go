// Package checksum is Evo's one content-identity engine. File.Checksum,
// every leaf inside Tree.Checksum, File/Tree Equal, and Task Basis all
// reach content through Engine, so a digest means the same thing wherever
// it appears.
//
// A file's digest is the SHA-256 of its bytes. A tree's digest is a
// Merkle digest over each directory's entries sorted by name, each entry
// framed as (kind, name, child digest), so structure, names, contents, and
// whether each regular file is executable all count, while the tree's own
// location, timestamps, owners, and other mode bits do not. Symlinks count
// by their link text, never followed.
package checksum

import (
	"crypto/sha256"
	"encoding/hex"
)

// Digest is one SHA-256 identity.
type Digest [sha256.Size]byte

// String is the digest's public form: lowercase hex, 64 characters.
func (d Digest) String() string { return hex.EncodeToString(d[:]) }
