package fingerprint

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Type markers distinguish otherwise-identical byte preimages (a symlink
// whose target text happens to match a file's contents, an empty directory
// vs. a missing path) so no two semantically different observations can
// collide on digest.
const (
	markerRegularFile = "evident-output:fspath:file:v1\x00"
	markerSymlink     = "evident-output:fspath:symlink:v1\x00"
	markerMissing     = "evident-output:fspath:missing:v1"
	markerDirEntry    = "evident-output:fspath:dir-entry:v1\x00"
	markerDir         = "evident-output:fspath:dir:v1\x00"
)

// fsPathFingerprint implements Fingerprint for FSPath(path).
type fsPathFingerprint struct {
	path string
}

// FSPath fingerprints the content at path (spec §11.1):
//
//   - regular file: SHA-256 of a type marker plus the file's bytes;
//   - directory: a deterministic Merkle digest over sorted relative entry
//     names, entry types, symlink targets, and file bytes;
//   - symlink: the link target text, never followed;
//   - missing path: a stable digest distinct from any present path;
//   - anything else (permission denied, I/O error): an error, never
//     reported as "missing".
//
// Ordinary permission/mtime/owner changes never change the digest.
func FSPath(path string) Fingerprint {
	return fsPathFingerprint{path: path}
}

func (f fsPathFingerprint) Fingerprint(_ context.Context) (FingerprintValue, error) {
	digest, err := fingerprintPath(activeFS, f.path)
	if err != nil {
		return FingerprintValue{}, fmt.Errorf("fingerprint fs path %q: %w", f.path, err)
	}
	return FingerprintValue{Kind: KindFSPath, Key: f.path, Digest: digest}, nil
}

// fingerprintPath is FSPath's observation, factored out so directory
// traversal (fingerprintDir) can recurse into it for each entry.
func fingerprintPath(vfs FS, path string) (Digest, error) {
	info, err := vfs.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return sum256([]byte(markerMissing)), nil
	}
	if err != nil {
		return Digest{}, err
	}
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		target, err := vfs.Readlink(path)
		if err != nil {
			return Digest{}, err
		}
		return sum256(append([]byte(markerSymlink), target...)), nil
	case info.IsDir():
		return fingerprintDir(vfs, path)
	case info.Mode().IsRegular():
		contents, err := vfs.ReadFile(path)
		if err != nil {
			return Digest{}, err
		}
		return sum256(append([]byte(markerRegularFile), contents...)), nil
	default:
		return Digest{}, fmt.Errorf("fs path %q is neither a regular file, directory, nor symlink", path)
	}
}

// fingerprintDir computes the deterministic recursive Merkle digest for a
// directory: sorted entry names, each entry's own digest (recursing through
// fingerprintPath so nested files/dirs/symlinks use identical rules), and
// each entry's type — mtimes/permissions/ownership never enter the hash.
func fingerprintDir(vfs FS, path string) (Digest, error) {
	entries, err := vfs.ReadDir(path)
	if err != nil {
		return Digest{}, err
	}
	names := sortedDirEntryNames(entries)
	preimage := []byte(markerDir)
	for _, name := range names {
		childDigest, err := fingerprintPath(vfs, filepath.Join(path, name))
		if err != nil {
			return Digest{}, fmt.Errorf("entry %q: %w", name, err)
		}
		preimage = append(preimage, markerDirEntry...)
		preimage = append(preimage, name...)
		preimage = append(preimage, 0)
		preimage = append(preimage, childDigest[:]...)
	}
	return sum256(preimage), nil
}
