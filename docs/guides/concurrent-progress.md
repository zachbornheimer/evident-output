# Concurrent progress (WS-6 guidance)

## Predeclare (RULE-004)

```go
for path, task := range out.Group("placement").Each(paths) {
    task.Define(func() error { return place(path) })
}
```

Do **not** spawn caller goroutines to make Group work parallel — Evo's scheduler owns overlap. `Sequence` is the same scheduler with implicit predecessor dependencies. `After(...)` is the escape hatch for a DAG edge nesting cannot express.

## Neutral domain boundary

```go
type PlaceCallbacks struct {
    OnPhase func(string)
    OnBytes func(completed, total int64)
}
// domain receives PlaceCallbacks, not *evo.TaskHandle
```

## Scale (RULE-005)

| Workload | Model                                               |
| -------- | --------------------------------------------------- |
| Small    | One Task per operation                              |
| Medium   | Aggregate Progress + optional active-transfer Group |
| Huge     | Aggregate counts + bounded failure Problems         |
| Dry-run  | Plan only                                           |

## Viewport ≠ model

Renderer may cap visible rows. Semantic Task set is durable; do not create/destroy
tasks only for terminal height.
