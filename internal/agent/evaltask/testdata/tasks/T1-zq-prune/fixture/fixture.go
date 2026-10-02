// Package fixture is the hermetic stand-in for zq's repository and package
// manager layers. Every call answers from memory.
package fixture

import "context"

// IncludeRemote reports whether the run was asked to prune remote branches.
func IncludeRemote() bool { return true }

// LandedBranches lists local branches already merged into the trunk.
func LandedBranches(context.Context) ([]string, error) { return []string{"old"}, nil }

// UnusedWorktrees lists worktrees no branch uses.
func UnusedWorktrees(context.Context) ([]string, error) { return []string{"/kept"}, nil }

// PruneStaleRemoteRefs drops remote-tracking refs the remote no longer has.
func PruneStaleRemoteRefs(context.Context) error { return nil }

// DeleteRemoteBranches deletes the given branches on the remote.
func DeleteRemoteBranches(context.Context, []string) error { return nil }

// DetectPackageManagers names the package managers used by the worktrees.
func DetectPackageManagers(context.Context, []string) ([]string, error) {
	return []string{"composer", "npm"}, nil
}

// InstalledPackages maps each manager to the packages it has installed.
func InstalledPackages(context.Context) (map[string][]string, error) {
	return map[string][]string{
		"composer": {"psr/log@3.0.0"},
		"npm":      {"debug@4.3.4", "lodash@4.17.21"},
	}, nil
}

// Centralize moves one installed package into the shared store.
func Centralize(context.Context, string) error { return nil }
