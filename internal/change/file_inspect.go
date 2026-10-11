// This file owns how File inspects a path read-only and what it finds there:
// the safe Lstat, the content comparison, and the delta a reconcile must
// bring to disk.

package change

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"

	sysfs "github.com/zachbornheimer/evident-output/internal/fs"
)

// inheritedModeBits are the mode bits an unmanaged rewrite carries over:
// the permissions and setuid, setgid, and sticky, which Perm() alone
// would drop.
const inheritedModeBits = fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky

// ownerRead is the permission bit readExisting adds to open a file whose
// mode denies even its owner a read.
const ownerRead fs.FileMode = 0o400

// fileDelta is what reconciling one File must change on disk.
type fileDelta struct {
	exists      bool
	writeNeeded bool
	modeDiffers bool
	// writeMode is the permission a content write must leave: the managed
	// Mode, else the existing file's own, else sysfs.UnmanagedCreateMode.
	writeMode fs.FileMode
	// ordinaryCreate is a new file whose mode is unmanaged: the umask
	// decides its permission.
	ordinaryCreate bool
}

func (d fileDelta) mutates() bool { return d.writeNeeded || d.modeDiffers }

// inspectFile observes op's path read-only and reports what reconciling it
// must change.
func (r *Run) inspectFile(fsys sysfs.FS, op fileOperation) (fileDelta, error) {
	spec, path := op.spec, op.path
	info, exists, statErr := inspectFilePath(fsys, path)
	if statErr != nil {
		return fileDelta{}, statErr
	}
	r.host.Observe(Observation{Task: op.taskID, Kind: ResourceObserved, Subject: Subject{Kind: SubjectFile, Name: path}, Exists: exists})

	if !exists && !op.contentsManaged() {
		return fileDelta{}, fmt.Errorf("%w: %s", ErrFileUnmanagedContentsMissing, path)
	}
	writeNeeded, readErr := fileNeedsContentWrite(fsys, path, exists, op.contentsManaged(), spec.Contents)
	if readErr != nil {
		return fileDelta{}, fmt.Errorf("evo: File inspect %q: %w", path, readErr)
	}
	modeDiffers := spec.Mode != 0 && (!exists || info.Mode().Perm() != spec.Mode.Perm())
	return fileDelta{
		exists:         exists,
		writeNeeded:    writeNeeded,
		modeDiffers:    modeDiffers,
		writeMode:      contentWriteMode(spec.Mode, info, exists),
		ordinaryCreate: !exists && spec.Mode == 0,
	}, nil
}

// contentWriteMode is the permission a content write leaves: the managed
// mode when there is one, otherwise the existing file's own (an unmanaged
// rewrite never changes it), otherwise ordinary creation semantics.
func contentWriteMode(managed fs.FileMode, info fs.FileInfo, exists bool) fs.FileMode {
	switch {
	case managed != 0:
		return managed
	case exists:
		return info.Mode() & inheritedModeBits
	default:
		return sysfs.UnmanagedCreateMode
	}
}

// inspectFilePath Lstats path without following a symlink and rejects
// anything File cannot own: a symlink, a directory, or another
// non-regular type. A missing path is not an error.
func inspectFilePath(fsys sysfs.FS, path string) (info fs.FileInfo, exists bool, err error) {
	info, statErr := fsys.Lstat(path)
	switch {
	case statErr == nil && info.Mode()&fs.ModeSymlink != 0:
		return nil, false, fmt.Errorf("%w: %s", ErrFilePathIsSymlink, path)
	case statErr == nil && info.IsDir():
		return nil, false, fmt.Errorf("%w: %s is a directory", ErrFilePathTypeMismatch, path)
	case statErr == nil && !info.Mode().IsRegular():
		return nil, false, fmt.Errorf("%w: %s", ErrFilePathTypeMismatch, path)
	case statErr != nil && !errors.Is(statErr, fs.ErrNotExist):
		return nil, false, fmt.Errorf("evo: File inspect %q: %w", path, statErr)
	}
	return info, statErr == nil, nil
}

// fileNeedsContentWrite reports whether Contents must be (re)written for
// the file to match spec: unmanaged Contents never triggers a write; a
// missing file with managed Contents always does; an existing file needs
// one only when its current bytes differ from the desired ones.
func fileNeedsContentWrite(fsys sysfs.FS, path string, exists, contentsManaged bool, desired []byte) (bool, error) {
	if !contentsManaged {
		return false, nil
	}
	if !exists {
		return true, nil
	}
	current, err := readExisting(fsys, path)
	if err != nil {
		return false, err
	}
	return !bytes.Equal(current, desired), nil
}

// readExisting reads path. A mode of 0000 is unreadable, so an existing
// file that denies the read is opened by adding owner-read, then its mode
// is put back before the caller decides whether to rewrite. The rewrite
// itself still uses the original mode, so an unmanaged 0000 stays 0000.
func readExisting(fsys sysfs.FS, path string) ([]byte, error) {
	current, err := fsys.ReadFile(path)
	if err == nil || !errors.Is(err, fs.ErrPermission) {
		return current, err
	}
	info, statErr := fsys.Lstat(path)
	if statErr != nil {
		return nil, err
	}
	original := info.Mode() & inheritedModeBits
	if chmodErr := fsys.Chmod(path, original|ownerRead); chmodErr != nil {
		return nil, err
	}
	current, readErr := fsys.ReadFile(path)
	if chmodErr := fsys.Chmod(path, original); chmodErr != nil && readErr == nil {
		return nil, chmodErr
	}
	return current, readErr
}
