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
  debug-history
  debug-pane
)

# Extra args per example.
# live-progress on a TTY: real in-place ANSI live region (kitty / Terminal.app).
# When stderr is not a TTY (CI/logs) or EVO_EXAMPLES_FRAMES=1: numbered --frames dump.
# Force scrubable frames: EVO_EXAMPLES_FRAMES=1 mise run examples
example_args() {
  case "$1" in
    live-progress)
      # Still uses demo.Options color flag until terminal-driver split.
      if [[ "${EVO_EXAMPLES_FRAMES:-}" == "1" ]] || [[ ! -t 2 ]]; then
        echo "--fast --frames --color=always"
      else
        echo "--color=always"
      fi
      ;;
    repo-status)
      echo "--fast --color=auto"
      ;;
    doctor|debug-history|debug-pane|install-pipeline|migrate|data-command)
      echo "--fast"
      ;;
    *) echo "" ;;
  esac
}

BIN_DIR="$(mktemp -d "${TMPDIR:-/tmp}/evo-examples.XXXXXX")"
cleanup() { rm -rf "${BIN_DIR}"; }
trap cleanup EXIT

hr() {
  local name="$1"
  local args="$2"
  printf '\n'
  printf '════════════════════════════════════════════════════════════\n'
  printf '  example: %s\n' "${name}"
  printf '  go run ./examples/%s/ %s\n' "${name}" "${args}"
  if [[ "${name}" == "live-progress" && -t 2 ]]; then
    printf '  mode: live ANSI region on stderr (in-place redraw)\n'
  elif [[ "${name}" == "live-progress" ]]; then
    printf '  mode: --frames (stderr is not a TTY)\n'
  fi
  printf '════════════════════════════════════════════════════════════\n'
  printf '\n'
}

run_one() {
  local name="$1"
  local args="$2"
  local dir="${ROOT}/examples/${name}"
  if [[ ! -d "${dir}" ]]; then
    echo "missing example dir: ${dir}" >&2
    exit 1
  fi
  hr "${name}" "${args}"
  local bin="${BIN_DIR}/${name}"
  go build -o "${bin}" "./examples/${name}/"
  set +e
  # shellcheck disable=SC2086
  "${bin}" ${args}
  local code=$?
  set -e
  if [[ "${code}" -ne 0 ]]; then
    printf '\n(exit %s — conclusion exit code from the demo)\n' "${code}"
  fi
}

for name in "${EXAMPLES[@]}"; do
  args="$(example_args "${name}")"
  run_one "${name}" "${args}"
  # Second pass for pane: show failure diagnostic tail (§21.3.2).
  if [[ "${name}" == "debug-pane" ]]; then
    run_one "debug-pane" "--fast --fail"
  fi
done

printf '\n'
printf '════════════════════════════════════════════════════════════\n'
printf '  examples complete (%d + debug-pane --fail)\n' "${#EXAMPLES[@]}"
printf '  tip: go run ./examples/live-progress/           # live in-place (TTY)\n'
printf '       go run ./examples/debug-history/           # debug above live\n'
printf '       go run ./examples/debug-pane/              # rolling pane\n'
printf '       go run ./examples/debug-pane/ --fail       # diagnostics tail\n'
printf '       go run ./examples/doctor/ --json | jq .conclusion\n'
printf '════════════════════════════════════════════════════════════\n'
