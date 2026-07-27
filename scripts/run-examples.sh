#!/usr/bin/env bash
# Run example mini-CLIs back-to-back with section headers (mise run examples).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

# Each entry is a real small program under examples/<name>/ — useful as copy-paste shape.
EXAMPLES=(
  repo-status
  install-pipeline
  migrate
  doctor
  data-command
  live-progress
)

# Extra args per example.
# live-progress: --frames so each redraw is a numbered scrubable snapshot
# (in-place ANSI is for an interactive TTY: go run ./examples/live-progress/).
example_args() {
  case "$1" in
    live-progress) echo "--fast --frames --color=always" ;;
    *) echo "--color=always" ;;
  esac
}

BIN_DIR="$(mktemp -d "${TMPDIR:-/tmp}/evo-examples.XXXXXX")"
cleanup() { rm -rf "${BIN_DIR}"; }
trap cleanup EXIT

hr() {
  printf '\n'
  printf '════════════════════════════════════════════════════════════\n'
  printf '  example: %s\n' "$1"
  printf '  go run ./examples/%s/\n' "$1"
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
  # Build then exec so Conclusion exit codes are preserved (go run collapses them).
  # --color=always so demos show SGR even when the agent/CI shell exports NO_COLOR.
  bin="${BIN_DIR}/${name}"
  go build -o "${bin}" "./examples/${name}/"
  set +e
  # shellcheck disable=SC2046
  "${bin}" $(example_args "${name}")
  code=$?
  set -e
  if [[ "${code}" -ne 0 ]]; then
    printf '\n(exit %s — conclusion exit code from the demo)\n' "${code}"
  fi
done

printf '\n'
printf '════════════════════════════════════════════════════════════\n'
printf '  examples complete (%d)\n' "${#EXAMPLES[@]}"
printf '  tip: go run ./examples/doctor/ --json | jq .conclusion\n'
printf '       go run ./examples/data-command/ 2>/dev/null | jq .\n'
printf '       go run ./examples/migrate/ --apply\n'
printf '════════════════════════════════════════════════════════════\n'
