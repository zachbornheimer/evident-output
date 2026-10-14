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

# Exit codes evo.Main returns (README "Outcome and exit code"): 1 when a Task
# returned evo.Refuse, 2 when one failed. A Problem never ends a Task, so only
# an example that returns a refusal or a failure uses them. Every documented
# flag's exit code is pinned in examples/flag_behaviour_test.go.
EXIT_REFUSED=1
EXIT_FAILED=2

# Exit code of an example's default run: repo-status refuses while the
# repository has blockers.
default_exit() {
	case "$1" in
	repo-status) echo "${EXIT_REFUSED}" ;;
	*) echo 0 ;;
	esac
}

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
		printf '\n(exit %s, expected: this example demonstrates a refused or failed run)\n' "${code}"
	fi
}

for name in "${EXAMPLES[@]}"; do
	args="$(example_args "${name}")"
	run_one "${name}" "${args}" "$(default_exit "${name}")"
	case "${name}" in
	verbose) run_one verbose "--verbose" 0 ;;
	repo-status) run_one repo-status "--clean --fast" 0 ;;
	install-pipeline) run_one install-pipeline "--fast --fail-tests" "${EXIT_FAILED}" ;;
	migrate)
		run_one migrate "--apply" 0
		run_one migrate "--apply --fail" "${EXIT_FAILED}"
		;;
	doctor) run_one doctor "--fast --strict" "${EXIT_REFUSED}" ;;
	data-command) run_one data-command "--fail-link" "${EXIT_FAILED}" ;;
	debug-pane) run_one debug-pane "--fast --fail" "${EXIT_REFUSED}" ;;
	*) ;;
	esac
done

printf '\n'
printf '════════════════════════════════════════════════════════════\n'
printf '  examples complete (ladder: print → verbose → … → terminal-driver)\n'
printf '════════════════════════════════════════════════════════════\n'
