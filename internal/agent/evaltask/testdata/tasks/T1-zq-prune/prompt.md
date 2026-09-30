I'm writing the `prune` command for my repo-cleanup CLI. Write its `main.go`.

It cleans up in two areas.

The repository: delete branches that already landed, remove worktrees nobody uses, and drop stale remote-tracking refs. If remote pruning is on, also delete the branches that landed on the remote. That last step has to wait until the landed branches are known.

Then consolidate packages, in this order: work out which package managers the worktrees use (this must wait until the unused worktrees are removed), list the installed packages, then move every package into the shared store, one step per package, grouped by manager.

I want it to show live progress and report what it did. The domain helpers are already written in the package `evalsandbox/fixture`; call them, don't reimplement them.
