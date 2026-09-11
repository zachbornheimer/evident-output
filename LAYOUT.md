.: public evo package — type aliases and one-line wrappers over internal/engine
internal/engine: presentation engine (Output, tasks, confirm, evidence, print, run)
internal/core: domain types (Snapshot, Problem, Action, Event, state, conclusion)
internal/render: human, JSON, and live projection (one package; they share DisplayUnit)
internal/text: glyphs, sanitize, width, conjugate, name truncation
internal/agent: MCP tools (adopt, review, catalog, preview, sections, harness, rules)
internal/wireschema: JSON schema validation for the wire documents
terminal: ANSI terminal driver
testkit: test helpers (clock, screen, output assertions)
cmd/evident-output: CLI (review, adopt, explain)
cmd/evident-output-mcp: MCP server
examples: runnable demos (repo-status, doctor, data-command, …)
conformance: release gates, goldens, TRACEABILITY
schema: wire JSON schemas (event.v1, output.v1)
docs: guides, architecture, API inventory, ADRs
integrations: editor/agent install notes
skills: agent skill docs
tools/agents: engineer prompt (not MCP — that is internal/agent)
tools/scripts: release, traceability, usage-audit, pin sync
tools/gensections: embed docs into the MCP server
