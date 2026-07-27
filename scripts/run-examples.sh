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
# live-progress on a TTY: real in-place ANSI live region (kitty / Terminal.app).
# When stderr is not a TTY (CI/logs) or EVO_EXAMPLES_FRAMES=1: numbered --frames dump.
# Force scrubable frames: EVO_EXAMPLES_FRAMES=1 mise run examples
example_args() {
  case "$1" in
    live-progress)
      if [[ "${EVO_EXAMPLES_FRAMES:-}" == "1" ]] || [[ ! -t 2 ]]; then
        echo "--fast --frames --color=always"
      else
        # In-place live region; no --frames. Default 100ms steps (not --fast).
        echo "--color=always"
      fi
      ;;
    # Progressive item demos: short sleeps so the batch still feels real-time.
    repo-status|doctor)
      echo "--fast --color=always"
      ;;
    *) echo "--color=always" ;;
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

for name in "${EXAMPLES[@]}"; do
  dir="${ROOT}/examples/${name}"
  if [[ ! -d "${dir}" ]]; then
    echo "missing example dir: ${dir}" >&2
    exit 1
  fi
  args="$(example_args "${name}")"
  hr "${name}" "${args}"
  # Build then exec so Conclusion exit codes are preserved (go run collapses them).
  bin="${BIN_DIR}/${name}"
  go build -o "${bin}" "./examples/${name}/"
  set +e
  # shellcheck disable=SC2086
  "${bin}" ${args}
  code=$?
  set -e
  if [[ "${code}" -ne 0 ]]; then
    printf '\n(exit %s — conclusion exit code from the demo)\n' "${code}"
  fi
done

printf '\n'
printf '════════════════════════════════════════════════════════════\n'
printf '  examples complete (%d)\n' "${#EXAMPLES[@]}"
printf '  tip: go run ./examples/live-progress/           # live in-place (TTY)\n'
printf '       go run ./examples/live-progress/ --frames  # scrubable log\n'
printf '       go run ./examples/live-progress/ --step    # Enter between frames\n'
printf '       go run ./examples/doctor/ --json | jq .conclusion\n'
printf '════════════════════════════════════════════════════════════\n'
