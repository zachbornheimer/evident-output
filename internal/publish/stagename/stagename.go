// Package stagename is the one naming scheme for publish staging entries:
// a private sibling of the destination, shaped
// .evo-<owner>-<random>.tmp. The owner tag binds the entry to its
// destination's name; the random part is unpredictable, so no other user
// can plant a symlink at a staging name in advance. Readers and digests
// recognise only this exact shape, never a loose pattern, so a real entry
// with a similar name is always content.
package stagename

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

const (
	prefix = ".evo-"
	suffix = ".tmp"
	// ownerBytes is how much of the destination name's digest the owner
	// tag carries; ownerLen is its length in hex.
	ownerBytes = 4
	ownerLen   = 2 * ownerBytes
	// randomLen is the length of rand.Text: base32 (RFC 4648), lowercased.
	randomLen = 26
	// randomAlphabet is rand.Text's alphabet, lowercased.
	randomAlphabet = "abcdefghijklmnopqrstuvwxyz234567"
	hexDigits      = "0123456789abcdef"
	// nameLen is every staging name's length.
	nameLen = len(prefix) + ownerLen + 1 + randomLen + len(suffix)
)

// New is a fresh staging path beside dest, owned by dest.
func New(dest string) string {
	return filepath.Join(filepath.Dir(dest), Owner(dest)+strings.ToLower(rand.Text())+suffix)
}

// Owner is the name prefix every staging entry of dest shares.
func Owner(dest string) string {
	sum := sha256.Sum256([]byte(filepath.Base(dest)))
	return prefix + hex.EncodeToString(sum[:ownerBytes]) + "-"
}

// Is reports whether name has exactly the staging shape.
func Is(name string) bool {
	if len(name) != nameLen || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return false
	}
	owner, rest, ok := strings.Cut(name[len(prefix):len(name)-len(suffix)], "-")
	return ok && len(owner) == ownerLen && only(owner, hexDigits) &&
		len(rest) == randomLen && only(rest, randomAlphabet)
}

// only reports whether every byte of s is in alphabet.
func only(s, alphabet string) bool {
	for i := range len(s) {
		if !strings.ContainsRune(alphabet, rune(s[i])) {
			return false
		}
	}
	return true
}
