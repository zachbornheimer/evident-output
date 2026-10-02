#!/usr/bin/env bash
# Paid pit-of-success eval entrypoint (mise run eval -- FLAGS).
#
# A real run needs ANTHROPIC_API_KEY, --max-usd N and --confirm-spend.
# --dry-run prints the plan and worst-case cost and needs none of them.
# Put flags before task ids: mise run eval -- --max-usd 5 --confirm-spend --all-ready
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

dry_run=false
has_max_usd=false
has_confirm=false
for arg in "$@"; do
  case "$arg" in
  --dry-run | --dry-run=true) dry_run=true ;;
  --max-usd | --max-usd=* | -max-usd | -max-usd=*) has_max_usd=true ;;
  --confirm-spend | --confirm-spend=true | -confirm-spend | -confirm-spend=true) has_confirm=true ;;
  esac
done

refuse() {
  echo "eval: refusing to start: $1" >&2
  exit 1
}

if [[ "$dry_run" == false ]]; then
  [[ -n "${ANTHROPIC_API_KEY:-}" ]] || refuse "ANTHROPIC_API_KEY is not set"
  [[ "$has_max_usd" == true ]] || refuse "--max-usd is required (the hard spend cap in USD)"
  [[ "$has_confirm" == true ]] || refuse "--confirm-spend is required for a run that spends money"
fi

mcp_binary="$here/results/.bin/evident-output-mcp"
mkdir -p "$here/results/.bin"
(cd "$here/.." && go build -o "$mcp_binary" ./cmd/evident-output-mcp)
cd "$here"
exec go run ./cmd/run --mcp-binary "$mcp_binary" "$@"
