# Exit-code fidelity: propagating a child's own exit code

`evo.Main(run)` / `evo.MainWith(out, run)` own the process's exit entirely —
they compute the Conclusion's exit code (0/1/2/130) and exit themselves. That
contract does not change for a CLI that must also propagate a **child
process's own exit code** — e.g. a task runner (`go-task`, `make`-alikes)
whose own exit code must match the last command it ran, not just whether the
run's presentation reports OK/Failed.

## The pattern

Use `evo.Run` / `out.Run` (the non-exiting reconciler — see
[reference.md](../reference.md#construction-config-lifecycle)) to get evo's
own code, then override it only when a child process exited non-zero for its
own reason:

```go
func main() {
    out := evo.Init(evo.Config{Title: "runner"})

    var childErr *exec.ExitError
    code := evo.Run(func(o *evo.Output) error {
        cmd := o.Task("build")
        err := run(cmd, ...) // wires cmd.Stdout/Stderr into evo evidence
        errors.As(err, &childErr)
        return err
    })
    if childErr != nil {
        code = childErr.ExitCode()
    }
    os.Exit(code)
}
```

- `evo.Run` still owns Finish/Close and the presentation's own 0/1/2/130
  contract — nothing about that changes.
- `code` starts as evo's own exit code, and is overridden only when the
  actual failure reason is "the child exited N", so the process's exit code
  matches what a user invoking that child directly would have seen.
- `os.Exit(code)` is the only line touching `os` here; nothing else in the
  program calls `os.Exit` directly (API-018 still applies everywhere else).

## Review guidance (API-018)

`evident_output_review` recognizes this exact shape as GoodCode, not a
warning to work around: `os.Exit(x)` is allowed when `x` was ever assigned
the result of `evo.Run(...)` / `out.Run(...)` / a `.ExitCode` call anywhere
earlier in the function, even after a conditional override. A bare
`os.Exit(1)` that never touched `evo.Run`'s result is still flagged — the
allowance is specifically for the capture-then-override pattern above, never
a blanket pass on `os.Exit`.
