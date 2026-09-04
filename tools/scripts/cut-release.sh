#!/usr/bin/env bash
# cut-release — bump PublishedRelease, sync pins, tag, and push.
#
# Prerequisites: implementation is already committed (or will be committed with
# the pin bump). Prefer:
#
#   mise run test && mise run cut-release
#   mise run test && VERSION=v0.2.12 mise run cut-release
#
# Never moves an existing tag. Never force-pushes.
set -euo pipefail

root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root"

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "cut-release: not a git repository" >&2
  exit 1
fi

branch="$(git rev-parse --abbrev-ref HEAD)"
if [[ "${branch}" != "main" && "${CUT_RELEASE_ALLOW_BRANCH:-}" != "1" ]]; then
  echo "cut-release: must run on main (got ${branch}); set CUT_RELEASE_ALLOW_BRANCH=1 to override" >&2
  exit 1
fi

if [[ -n "$(git status --porcelain)" && "${CUT_RELEASE_ALLOW_DIRTY:-}" != "1" ]]; then
  # Allow only if we're about to commit pin files; still refuse unknown dirty.
  echo "cut-release: working tree dirty. Commit implementation first, or set CUT_RELEASE_ALLOW_DIRTY=1" >&2
  git status --short >&2
  exit 1
fi

current="$(sed -n 's/^const PublishedRelease = "\(v[^"]*\)"/\1/p' release.go | head -1)"
if [[ -z "${current}" ]]; then
  echo "cut-release: could not parse PublishedRelease from release.go" >&2
  exit 1
fi

next="${VERSION:-}"
if [[ -z "${next}" ]]; then
  # Auto patch bump: v0.2.11 → v0.2.12
  if [[ "${current}" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
    major="${BASH_REMATCH[1]}"
    minor="${BASH_REMATCH[2]}"
    patch="${BASH_REMATCH[3]}"
    next="v${major}.${minor}.$((patch + 1))"
  else
    echo "cut-release: current ${current} is not vMAJOR.MINOR.PATCH; pass VERSION=..." >&2
    exit 1
  fi
fi

if [[ ! "${next}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "cut-release: VERSION must be vMAJOR.MINOR.PATCH (got ${next})" >&2
  exit 1
fi

if git rev-parse -q --verify "refs/tags/${next}" >/dev/null 2>&1; then
  echo "cut-release: tag ${next} already exists — refuse to move it" >&2
  exit 1
fi

echo "cut-release: ${current} → ${next}"

# Bump PublishedRelease in release.go
tmp="$(mktemp)"
sed "s/^const PublishedRelease = \"v[^\"]*\"/const PublishedRelease = \"${next}\"/" release.go >"${tmp}"
mv "${tmp}" release.go

go run ./tools/scripts/sync-release-pins
go test . -run 'PublishedRelease|VersionDrift' -count=1

msg="${CUT_RELEASE_MESSAGE:-chore(${next}): cut release}"
git add release.go README.md docs/mcp.md skills integrations 2>/dev/null || true
# Stage any pin surface the syncer touched
git add -u README.md docs/mcp.md skills integrations release.go 2>/dev/null || true
if [[ -n "$(git status --porcelain)" ]]; then
  git commit -m "${msg}"
else
  echo "cut-release: no pin-file changes to commit (PublishedRelease already ${next}?)"
fi

git tag -a "${next}" -m "${next}"
echo "cut-release: tagged ${next} at $(git rev-parse --short HEAD)"

if [[ "${CUT_RELEASE_PUSH:-1}" == "1" ]]; then
  git push origin HEAD
  git push origin "refs/tags/${next}"
  echo "cut-release: pushed HEAD and ${next}"
else
  echo "cut-release: skip push (CUT_RELEASE_PUSH=0)"
fi

echo "cut-release: done ${next}"
