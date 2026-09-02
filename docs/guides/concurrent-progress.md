# Concurrent progress (WS-6 guidance)

## Predeclare (RULE-004)

```go
jobs := out.Tasks("placement")
type tracked struct {
    file File
    task *evo.TaskHandle
}
var tracked []tracked
for _, f := range sortedFiles {
    tracked = append(tracked, tracked{
        file: f,
        task: jobs.Task(f.RelPath, evo.ID("file."+stableID(f))),
    })
}
// start workers that only call task.Phase / Bytes / Done / Fail
```

Do **not** call `jobs.Task(...)` inside workers — declaration order is presentation order.

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
| Medium   | Aggregate Progress + optional active-transfer Tasks |
| Huge     | Aggregate counts + bounded failure Problems         |
| Dry-run  | Plan only                                           |

## Viewport ≠ model

Renderer may cap visible rows. Semantic Task set is durable; do not create/destroy
Tasks only for terminal height.
