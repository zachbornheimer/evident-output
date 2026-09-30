I'm writing the `serve` command for my agent-management CLI. Write its `main.go`.

It runs the daemon until I press Ctrl-C. The daemon writes its log lines as it runs and those lines must show up under its progress row. When I stop it, it must shut down cleanly and the report must say it was stopped on request, not that it failed. Show when the daemon is ready to serve.

I want live progress and a clear report. The domain helpers are in the package `evalsandbox/fixture`; call them, don't reimplement them.
