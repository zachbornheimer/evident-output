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

# Exit code evo.Main returns for a blocked run (README "Outcome and exit code").
EXIT_BLOCKED=2

# Flags each example accepts; passing one it does not define exits 2.
example_args() {
	case "$1" in
	repo-status) echo "--fast --color=auto" ;;
	doctor | debug-history | debug-pane | install-pipeline | live-progress)
		echo "--fast"
		;;
	terminal-driver) echo "--fast --frames" ;;
	*) echo "" ;; # print, verbose, migrate, data-command: default pass takes no flags
	esac
}

# Examples that demonstrate a blocked run on purpose exit EXIT_BLOCKED;
# every other pass must exit 0.
example_expected_exit() {
	case "$1" in
	repo-status | doctor) echo "${EXIT_BLOCKED}" ;;
	*) echo 0 ;;
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
	local want="$3"
	local dir="${ROOT}/examples/${name}"
	if [[ ! -d "${dir}" ]]; then
		echo "missing example dir: ${dir}" >&2
		exit 1
	fi
	hr "${name}" "${args}"
	local bin="${BIN_DIR}/${name}"
	go build -o "${bin}" "./examples/${name}/"
	local code=0
	# shellcheck disable=SC2086
	"${bin}" ${args} || code=$?
	if [[ "${code}" -ne "${want}" ]]; then
		printf '\nexample %s exited %s, expected %s\n' "${name}" "${code}" "${want}" >&2
		exit 1
	fi
	if [[ "${code}" -ne 0 ]]; then
		printf '\n(exit %s, expected: this example demonstrates a blocked run)\n' "${code}"
	fi
}

for name in "${EXAMPLES[@]}"; do
	args="$(example_args "${name}")"
	run_one "${name}" "${args}" "$(example_expected_exit "${name}")"
	if [[ "${name}" == "verbose" ]]; then
		run_one "verbose" "--verbose" 0
	fi
	if [[ "${name}" == "debug-pane" ]]; then
		run_one "debug-pane" "--fast --fail" "${EXIT_BLOCKED}"
	fi
done

printf '\n'
printf '════════════════════════════════════════════════════════════\n'
printf '  examples complete (ladder: print → verbose → … → terminal-driver)\n'
printf '════════════════════════════════════════════════════════════\n'
