# Evident Output

A Go presentation library for CLI state, progress, evidence, and conclusions.

```go
out := evo.For("bpp-csharp")
defer out.Close()

out.Item("working tree").OK()
out.Item("branches").Block("local-only branch")
return out.Finish()
```

Application code owns execution. `evo` owns presentation only.

## Install

```bash
go get github.com/zachbornheimer/evident-output
```

## Develop

```bash
mise run setup
mise run test
mise run ci
```

Conformance (roast) lives under `conformance/` and is the executable specification.
Architecture: `docs/architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.3.md`.

## License

Apache License 2.0
