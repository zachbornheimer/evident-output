#!/usr/bin/env bash
# Run example mini-CLIs back-to-back with section headers (mise run examples).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT}"

# Learning ladder (also documented in README).
EXAMPLES=(
  print
  verbose
  repo-status
  install-pipeline
  migrate
  doctor
  data-command
  live-progress
  debug-history
  debug-pane
  terminal-driver
)

example_args() {
  case "$1" in
  print) echo "" ;;
  verbose) echo "" ;; # second pass with --verbose below
  repo-status) echo "--fast --color=auto" ;;
  doctor | debug-history | debug-pane | install-pipeline | migrate | data-command | live-progress)
    echo "--fast"
    ;;
  terminal-driver)
    if [[ ! -t 2 ]]; then
      echo "--fast --frames"
    else
      echo "--fast --frames"
    fi
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
  if [[ "${name}" == "verbose" ]]; then
    run_one "verbose" "--verbose"
  fi
  if [[ "${name}" == "debug-pane" ]]; then
    run_one "debug-pane" "--fast --fail"
  fi
done

printf '\n'
printf '════════════════════════════════════════════════════════════\n'
printf '  examples complete (ladder: print → verbose → … → terminal-driver)\n'
printf '════════════════════════════════════════════════════════════\n'
