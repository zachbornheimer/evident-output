// This file owns the one naming scheme for publish staging entries: a
// private sibling of the destination, shaped .evo-<owner>-<random>.tmp. The
// owner tag binds the entry to its destination's name; the random part is
// unpredictable, so no other user can plant a symlink at a staging name in
// advance. Readers and digests recognise only this exact shape, never a loose
// pattern, so a real entry with a similar name is always content.

package change

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

const (
	stagingPrefix = ".evo-"
	stagingSuffix = ".tmp"
	// ownerBytes is how much of the destination name's digest the owner
	// tag carries; ownerLen is its length in hex.
	ownerBytes = 4
	ownerLen   = 2 * ownerBytes
	// randomLen is the length of rand.Text: base32 (RFC 4648), lowercased.
	randomLen = 26
	// randomAlphabet is rand.Text's alphabet, lowercased.
	randomAlphabet = "abcdefghijklmnopqrstuvwxyz234567"
	hexDigits      = "0123456789abcdef"
	// stagingNameLen is every staging name's length.
	stagingNameLen = len(stagingPrefix) + ownerLen + 1 + randomLen + len(stagingSuffix)
)

// stagingName is a fresh staging path beside dest, owned by dest.
func stagingName(dest string) string {
	return filepath.Join(filepath.Dir(dest), stagingOwner(dest)+strings.ToLower(rand.Text())+stagingSuffix)
}

// stagingOwner is the name prefix every staging entry of dest shares.
func stagingOwner(dest string) string {
	sum := sha256.Sum256([]byte(filepath.Base(dest)))
	return stagingPrefix + hex.EncodeToString(sum[:ownerBytes]) + "-"
}

// IsStaging reports whether name is exactly a publish staging entry.
// Readers that enumerate a directory (Find, Tree.Read) and tree digests
// skip these: they are another writer's uncommitted state, never part of
// the tree.
func IsStaging(name string) bool {
	if len(name) != stagingNameLen || !strings.HasPrefix(name, stagingPrefix) || !strings.HasSuffix(name, stagingSuffix) {
		return false
	}
	owner, rest, ok := strings.Cut(name[len(stagingPrefix):len(name)-len(stagingSuffix)], "-")
	return ok && len(owner) == ownerLen && consistsOf(owner, hexDigits) &&
		len(rest) == randomLen && consistsOf(rest, randomAlphabet)
}

// consistsOf reports whether every byte of s is in alphabet.
func consistsOf(s, alphabet string) bool {
	for i := range len(s) {
		if !strings.ContainsRune(alphabet, rune(s[i])) {
			return false
		}
	}
	return true
}
