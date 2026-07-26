# Security Policy

## Reporting

Report vulnerabilities privately to the maintainers. Do not open public issues
for active exploits involving terminal injection or secret leakage.

## Model

- Untrusted text is sanitized before display and journal emission.
- `Detail` is user-visible; `Cause` is diagnostic and redacted by default in human output.
- The library does not execute shell commands or open network connections.
- MCP (when enabled) reserves stdout for protocol traffic and does not mutate caller files in v1.
