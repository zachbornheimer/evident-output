#!/usr/bin/env bash
# Run example CLIs back-to-back with section headers (mise run examples).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

# Order is intentional: items → tasks → plan/changes → doctor mix → machine JSON → adapters.
EXAMPLES=(
  repository-item
  tasks-progress
  plan-changes
  doctor-items
  json-snapshot
  framework-adapters
)

BIN_DIR="$(mktemp -d "${TMPDIR:-/tmp}/evo-examples.XXXXXX")"
cleanup() { rm -rf "${BIN_DIR}"; }
trap cleanup EXIT

hr() {
  printf '\n'
  printf '════════════════════════════════════════════════════════════\n'
  printf '  example: %s\n' "$1"
  printf '════════════════════════════════════════════════════════════\n'
  printf '\n'
}

for name in "${EXAMPLES[@]}"; do
  dir="${ROOT}/examples/${name}"
  if [[ ! -d "${dir}" ]]; then
    echo "missing example dir: ${dir}" >&2
    exit 1
  fi
  hr "${name}"
  bin="${BIN_DIR}/${name}"
  go build -o "${bin}" "./examples/${name}/"
  # Non-zero exits are part of the demo (blocked/failed conclusions).
  set +e
  "${bin}"
  code=$?
  set -e
  if [[ "${code}" -ne 0 ]]; then
    printf '\n(exit %s — expected for demos with blocked/failed conclusions)\n' "${code}"
  fi
done

printf '\n'
printf '════════════════════════════════════════════════════════════\n'
printf '  examples complete (%d)\n' "${#EXAMPLES[@]}"
printf '════════════════════════════════════════════════════════════\n'
