#!/usr/bin/env bash
# The mechanical half of one hill-climb step. The edit itself comes from an
# evor/agent worker that the orchestrator runs; nothing here spawns an agent.
#
#   hillclimb.sh standing TRANSCRIPT...          training standing as JSON
#   hillclimb.sh check BASE                      guards (frozen files, tuned
#                                                surface, string-only Go edits,
#                                                task nouns), then mise run ci
#   hillclimb.sh decide BEFORE.json AFTER.json   exit 0 keep the edit, 1 revert
#
# One step: (1) run the training tasks, (2) hand failing transcripts to the
# worker for ONE edit to the tuned surface, (3) check BASE, (4) run the training
# tasks again, (5) decide; revert with git on exit 1. Stop when held-out shows
# no gain on 2 consecutive checks, training pass^3 is 100%, or the dollar cap
# is hit (see eval/hillclimb/loop.go for the same rules as code).
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
repo="$(cd "$here/.." && pwd)"
usage="usage: hillclimb.sh standing TRANSCRIPT... | check BASE | decide BEFORE.json AFTER.json"
command="${1:?$usage}"
shift

case "$command" in
standing | decide)
  cd "$here" && exec go run ./cmd/hillclimb "$command" "$@"
  ;;
check)
  base="${1:?$usage}"
  (cd "$here" && go run ./cmd/hillclimb guard "$base" "$repo")
  cd "$repo" && exec mise run ci
  ;;
*)
  echo "$usage" >&2
  exit 2
  ;;
esac
