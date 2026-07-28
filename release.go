package evo

// PublishedRelease is the current published module/MCP pin for install docs,
// skills, and MCP config-generator fallback when ldflags Version is "dev".
//
// Update this string when cutting a release. version_drift_test.go fails if
// portable guidance still recommends a different pin or @latest.
const PublishedRelease = "v0.2.10"
