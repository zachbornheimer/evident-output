package evo

// PublishedRelease is the single source of truth for the current published
// module and MCP pin used in install guidance.
//
// Maintenance class this protects (v0.2.10 hygiene, generalized):
//
//   - skills / integrations / README disagree on which tag to install
//   - MCP config generator falls back to a stale hardcoded tag
//   - portable docs recommend @latest or a personal-machine clone path
//   - signed tags ship with stale README pins (next patch, never rewrite history)
//
// When cutting a release:
//  1. Set PublishedRelease to the new tag (e.g. "v0.2.11").
//  2. Run: go run ./scripts/sync-release-pins
//  3. Run: go test . -run VersionDrift
//  4. Tag that commit; do not move prior tags.
//
// version_drift_test.go enforces the portable surface stays synchronized.
const PublishedRelease = "v0.4.1"
