#!/usr/bin/env bash
# cut-release — bump PublishedRelease, sync pins, promote CHANGELOG, tag, and push.
#
# Prerequisites: implementation is already committed (or will be committed with
# the pin bump). Prefer:
#
#   mise run test && mise run cut-release
#   mise run test && VERSION=v0.2.12 mise run cut-release
#
# Never moves an existing tag. Never force-pushes. Tags the root module, then
# the mcp module one commit later with its local replace dropped (see
# "Release order" below).
#
# CHANGELOG hygiene: the in-flight section is headed "## X.Y.Z (unreleased)".
# This script rewrites it to "## [X.Y.Z] - YYYY-MM-DD" (CUT_RELEASE_DATE
# overrides today's date) and stages CHANGELOG.md with the pin commit. It
# refuses to tag if the changelog has neither that section nor a released
# "## [X.Y.Z]" heading (set CUT_RELEASE_ALLOW_CHANGELOG_DRIFT=1 to override).
set -euo pipefail

root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root"

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	echo "cut-release: not a git repository" >&2
	exit 1
fi

branch="$(git rev-parse --abbrev-ref HEAD)"
if [[ "$branch" != "main" && "${CUT_RELEASE_ALLOW_BRANCH:-}" != "1" ]]; then
	echo "cut-release: must run on main (got ${branch}); set CUT_RELEASE_ALLOW_BRANCH=1 to override" >&2
	exit 1
fi

if [[ -n "$(git status --porcelain)" && "${CUT_RELEASE_ALLOW_DIRTY:-}" != "1" ]]; then
	# Allow only if we're about to commit pin files; still refuse unknown dirty.
	echo "cut-release: working tree dirty. Commit implementation first, or set CUT_RELEASE_ALLOW_DIRTY=1" >&2
	git status --short >&2
	exit 1
fi

release_file="internal/project/wire/stream.go"
current="$(sed -n 's/^const Release = "\(v[^"]*\)"/\1/p' "$release_file" | head -1)"
if [[ -z "$current" ]]; then
	echo "cut-release: could not parse Release from $release_file" >&2
	exit 1
fi

next="${VERSION:-}"
if [[ -z "$next" ]]; then
	# Auto patch bump: v0.2.11 → v0.2.12
	if [[ "$current" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
		major="${BASH_REMATCH[1]}"
		minor="${BASH_REMATCH[2]}"
		patch="${BASH_REMATCH[3]}"
		next="v${major}.${minor}.$((patch + 1))"
	else
		echo "cut-release: current ${current} is not vMAJOR.MINOR.PATCH; pass VERSION=..." >&2
		exit 1
	fi
fi

if [[ ! "$next" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	echo "cut-release: VERSION must be vMAJOR.MINOR.PATCH (got ${next})" >&2
	exit 1
fi

if git rev-parse -q --verify "refs/tags/mcp/${next}" >/dev/null 2>&1; then
	echo "cut-release: tag mcp/${next} already exists — refuse to move it" >&2
	exit 1
fi

if git rev-parse -q --verify "refs/tags/${next}" >/dev/null 2>&1; then
	echo "cut-release: tag ${next} already exists — refuse to move it" >&2
	exit 1
fi

echo "cut-release: ${current} → ${next}"

# --- CHANGELOG hygiene -------------------------------------------------------
# Bare version for Keep-a-Changelog headings: v0.5.0 → 0.5.0
bare="${next#v}"
changelog="CHANGELOG.md"
unreleased_heading="## ${bare} (unreleased)"
release_date="${CUT_RELEASE_DATE:-$(date +%Y-%m-%d)}"
if [[ -f "$changelog" && "${CUT_RELEASE_ALLOW_CHANGELOG_DRIFT:-}" != "1" ]]; then
	if command grep -Eq "^## \\[${bare}\\]( |$)" "$changelog"; then
		:
	elif command grep -Fxq "$unreleased_heading" "$changelog"; then
		tmp_changelog="$(mktemp)"
		awk -v old="$unreleased_heading" -v new="## [${bare}] - ${release_date}" '$0 == old {print new; next} {print}' "$changelog" >"$tmp_changelog"
		mv "$tmp_changelog" "$changelog"
		echo "cut-release: CHANGELOG.md: ${unreleased_heading} → ## [${bare}] - ${release_date}"
	else
		echo "cut-release: CHANGELOG.md has neither '${unreleased_heading}' nor '## [${bare}]'." >&2
		echo "cut-release: name the in-flight section '${unreleased_heading}', then re-run." >&2
		echo "cut-release: override with CUT_RELEASE_ALLOW_CHANGELOG_DRIFT=1 only if intentional." >&2
		exit 1
	fi
fi

# Bump Release in the wire package
tmp="$(mktemp)"
sed "s/^const Release = \"v[^\"]*\"/const Release = \"${next}\"/" "$release_file" >"$tmp"
mv "$tmp" "$release_file"

go run ./tools/scripts/sync-release-pins
# The released API contract is the baseline TestChangelogCoversEveryAPIChange
# diffs the next unreleased section against.
cp testdata/api_golden.txt testdata/api_golden_released.txt
# mcp/docs/mcp.md is embedded into the MCP binary; regenerate or the staleness gate goes red
(cd mcp && go generate ./internal/agent/sections)
go test . -run 'PublishedRelease|VersionDrift' -count=1

msg="${CUT_RELEASE_MESSAGE:-chore(${next}): cut release}"
git add "$release_file" README.md mcp/docs/mcp.md mcp/internal/agent/sections/embedded mcp/skills mcp/integrations mcp/go.mod mcp/internal/buildversion/buildversion.go CHANGELOG.md testdata/api_golden_released.txt 2>/dev/null || true
# Stage any pin surface the syncer touched (and CHANGELOG if promoted in-tree)
git add -u README.md mcp/docs/mcp.md mcp/internal/agent/sections/embedded mcp/skills mcp/integrations mcp/go.mod mcp/internal/buildversion/buildversion.go "$release_file" CHANGELOG.md 2>/dev/null || true
if [[ -n "$(git status --porcelain)" ]]; then
	git commit -m "$msg"
else
	echo "cut-release: no pin-file changes to commit (Release already ${next}?)"
fi

# --- Release order -----------------------------------------------------------
# 1. Tag the root module and publish it: the mcp module cannot resolve its
#    dependency on the root until the Go proxy can fetch that tag.
# 2. In a following commit, drop mcp/go.mod's local replace and tidy it against
#    the published root. `go install <mcp>@version` refuses any module whose
#    go.mod holds a replace, so mcp/<version> points at this commit, not the
#    root's. The mcp module lives in mcp/, so Go resolves its versions from
#    mcp/-prefixed tags. Local development keeps working through go.work.
root_module="github.com/zachbornheimer/evident-output/v2"
push="${CUT_RELEASE_PUSH:-1}"

git tag -a "$next" -m "$next"
echo "cut-release: tagged ${next} at $(git rev-parse --short HEAD)"
if [[ "$push" == "1" ]]; then
	git push origin HEAD
	git push origin "refs/tags/${next}"
	echo "cut-release: pushed HEAD and ${next}"
fi

if ! (cd mcp && go mod edit -dropreplace="$root_module" && go mod tidy); then
	echo "cut-release: could not resolve ${root_module}@${next} for mcp/go.mod." >&2
	echo "cut-release: ${next} is tagged (pushed: ${push}). Once the Go proxy serves it, run in mcp/:" >&2
	echo "cut-release:   go mod edit -dropreplace=${root_module} && go mod tidy" >&2
	echo "cut-release: then commit mcp/go.mod and mcp/go.sum and tag mcp/${next}." >&2
	exit 1
fi
if command grep -Eq '^replace[[:space:]]|=>' mcp/go.mod; then
	echo "cut-release: mcp/go.mod still holds a replace directive; refusing to tag mcp/${next}" >&2
	exit 1
fi
git add mcp/go.mod mcp/go.sum
git commit -m "chore(mcp/${next}): resolve the published root module"
git tag -a "mcp/${next}" -m "mcp/${next}"
echo "cut-release: tagged mcp/${next} at $(git rev-parse --short HEAD)"

if [[ "$push" == "1" ]]; then
	git push origin HEAD
	git push origin "refs/tags/mcp/${next}"
	echo "cut-release: pushed HEAD and mcp/${next}"
else
	echo "cut-release: skip push (CUT_RELEASE_PUSH=0)"
fi

echo "cut-release: done ${next}"
