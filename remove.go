package evo

import "context"

// Remove deletes the file. An absent path is success; a directory at Path
// is ErrFilePathTypeMismatch and nothing is deleted.
func (f File) Remove(ctx context.Context) error { return errNotImplemented }

// Remove deletes the tree recursively. An absent path is success; a
// regular file at Path is ErrTreePathTypeMismatch and nothing is deleted.
func (t Tree) Remove(ctx context.Context) error { return errNotImplemented }
