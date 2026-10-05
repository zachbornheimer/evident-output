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

// PathOf reports the filesystem path an FSPath Fingerprint observes, and
// whether f is an FSPath at all. It exists for the engine package's
// producer/consumer freshness barrier (spec §11.6): the barrier must key on
// and wait for a path *before* calling Fingerprint (which reads the file),
// so it needs the path without performing that read — never for a caller
// to bypass Fingerprint's own read-only observation.
func PathOf(f Fingerprint) (path string, ok bool) {
	p, ok := f.(fsPathFingerprint)
	if !ok {
		return "", false
	}
	return p.path, true
}

func (f fsPathFingerprint) Fingerprint(_ context.Context) (FingerprintValue, error) {
	digest, err := fingerprintPath(activeFS, f.path)
	if err != nil {
		return FingerprintValue{}, fmt.Errorf("fingerprint fs path %q: %w", f.path, err)
	}
	return FingerprintValue{Kind: KindFSPath, Key: f.path, Digest: digest}, nil
}

// ObservedFile is the identity FSPath(path) observes when path is a
// regular file holding contents. It lets an operation that must read the
// bytes anyway (Patch deriving desired state) record the Basis of exactly
// the bytes it read, instead of a second read that could see a different
// file.
func ObservedFile(path string, contents []byte) FingerprintValue {
	return FingerprintValue{Kind: KindFSPath, Key: path, Digest: regularFileDigest(contents)}
}

// ObservedMissing is the identity FSPath(path) observes when path does not
// exist.
func ObservedMissing(path string) FingerprintValue {
	return FingerprintValue{Kind: KindFSPath, Key: path, Digest: missingDigest()}
}

func regularFileDigest(contents []byte) Digest {
	return sum256(append([]byte(markerRegularFile), contents...))
}

func missingDigest() Digest { return sum256([]byte(markerMissing)) }

// fingerprintPath is FSPath's observation, factored out so directory
// traversal (fingerprintDir) can recurse into it for each entry.
func fingerprintPath(vfs FS, path string) (Digest, error) {
	info, err := vfs.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return missingDigest(), nil
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
		contents, err := readRegular(vfs, path, info.Mode())
		if err != nil {
			return Digest{}, err
		}
		return regularFileDigest(contents), nil
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

// keptModeBits are the mode bits readRegular puts back: the permissions and
// setuid, setgid, and sticky.
const keptModeBits = fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky

// ownerRead is the permission bit readRegular adds to open a file whose
// mode denies even its owner a read.
const ownerRead fs.FileMode = 0o400

// readRegular reads a regular file. Mode 0000 denies the read even to the
// owner, so the real filesystem facade adds owner-read for the read and
// puts the mode back. A fake FS is left alone.
func readRegular(vfs FS, path string, mode fs.FileMode) ([]byte, error) {
	contents, err := vfs.ReadFile(path)
	if err == nil || !errors.Is(err, fs.ErrPermission) {
		return contents, err
	}
	if _, ok := vfs.(osFS); !ok {
		return nil, err
	}
	original := mode & keptModeBits
	if chmodErr := os.Chmod(path, original|ownerRead); chmodErr != nil {
		return nil, err
	}
	contents, readErr := vfs.ReadFile(path)
	if chmodErr := os.Chmod(path, original); chmodErr != nil && readErr == nil {
		return nil, chmodErr
	}
	return contents, readErr
}
