I need the `adopt` command for my repo-cleanup CLI. Write its `main.go`.

First find the repositories under management. Then adopt each one, as its own visible step named after the repository, so I can see which repo is being worked on and which one failed. The repositories are only known after the first step finishes.

I want live progress and a clear report at the end. The domain helpers are in the package `evalsandbox/fixture`; call them, don't reimplement them.
