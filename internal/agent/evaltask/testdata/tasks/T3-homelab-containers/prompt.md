I'm writing the `converge` command for my homelab CLI. Write its `main.go`.

For every container in my stack, make sure it is running with its current config. If a container is already running with the right config, leave it alone and say so. Otherwise take it down, regenerate its config, and bring it back up. Each container is its own visible step named after the container, so I can see which one is being worked on.

I want live progress and a clear report. The domain helpers are in the package `evalsandbox/fixture`; call them, don't reimplement them.
