# Evident Output (evo) usage inventory — 001 repo

Scanned repo: `TESTDATA_ROOT` (evident-output v0.4.2). Every heading below is a file that uses evo in some form; every fenced block is the verbatim enclosing function/type/var declaration for one or more usage sites within it.

# TESTDATA_ROOT/main.go

**via:** direct

```go
// RunTask starts and finishes a task directly through evo — a direct
// usage.
func RunTask(name string) {
	task := evo.Task("run %s", name)
	task.Done()
}
```

**via:** evo-typed signature

```go
// Wrapper holds a task handle produced elsewhere in the program — an
// evo-typed field on a type declaration.
type Wrapper struct {
	Task *evo.TaskHandle
}
```

**via:** evo-typed signature

```go
// LogTask records lifecycle info for a task handle without calling evo
// itself: its param references evo.TaskHandle, so it counts as evo-typed
// rather than direct, and it exercises a method receiver.
func (w *Wrapper) LogTask(t *evo.TaskHandle) {
	fmt.Println("logged", t)
}
```

**via:** evo-typed signature

```go
// activeTask is an evo-typed spec inside a grouped var block; label is not,
// so the whole block still qualifies as evo-typed on activeTask alone.
var (
	label      = "fixture"
	activeTask *evo.TaskHandle
)
```
