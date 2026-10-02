package publish

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/zachbornheimer/evident-output/internal/publish/cacheroot"
)

// The staging root holds stages prepared apart from their destinations,
// under the user's cache directory beside the lock files: outside every
// tree, so no tree's commit can carry a stage away while it fills.
const (
	stagingRootName = "evo/stage"
	stagingRootMode = 0o700
)

// stagingRoot is the staging root, created once per process; "" when the
// user has no cache directory or it cannot be created.
var stagingRoot = sync.OnceValue(func() string {
	base, err := cacheroot.Dir()
	if err != nil {
		return ""
	}
	root := filepath.Join(base, stagingRootName)
	if os.MkdirAll(root, stagingRootMode) != nil {
		return ""
	}
	return root
})

// StagesApart reports whether a stage for dest is prepared in the staging
// root: only when the root shares dest's volume, so the move beside dest
// at commit is one atomic rename. Otherwise the stage is prepared beside
// dest, where an ancestor's commit can carry it away (ErrStagedGone).
func StagesApart(dest string) bool {
	root := stagingRoot()
	return stagingApartAllowed() && root != "" && sameVolume(root, existingAncestor(filepath.Dir(dest)))
}

// existingAncestor is dir, or its nearest ancestor that exists.
func existingAncestor(dir string) string {
	at := dir
	for {
		if _, err := os.Stat(at); err == nil || filepath.Dir(at) == at {
			return at
		}
		at = filepath.Dir(at)
	}
}

// lockBeside takes dest's lock for the commit. A stage prepared apart has
// its parent directories made first (the lock needs them) and is moved
// beside dest under the lock, so no ancestor commit can carry it away
// before the swap, and a crash from here on leaves it where Recover looks.
func (s *Staged) lockBeside(ctx context.Context) (*Hold, error) {
	if s.apart {
		created, err := makeParents(filepath.Dir(s.dest))
		if err != nil {
			return nil, err
		}
		s.created = created
	}
	hold, err := Lock(ctx, s.dest)
	if err != nil {
		return nil, err
	}
	if err := s.bringBeside(); err != nil {
		return nil, errors.Join(err, hold.Release())
	}
	return hold, nil
}

// bringBeside moves a stage prepared apart to a fresh staging name beside
// dest, re-leasing it under that name. Run it only under dest's lock.
func (s *Staged) bringBeside() error {
	if !s.apart {
		return nil
	}
	beside := stagingName(s.dest)
	lease, ok, err := tryLease(beside)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("staging entry %s is already leased", beside)
	}
	if err := os.Rename(s.temp, beside); err != nil {
		return errors.Join(fmt.Errorf("move stage %s beside %s: %w", s.temp, s.dest, err), lease.release())
	}
	err = s.lease.release()
	s.temp, s.lease, s.apart = beside, lease, false
	return err
}
