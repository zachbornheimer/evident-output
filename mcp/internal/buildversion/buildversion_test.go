package buildversion

import (
	"runtime/debug"
	"testing"
)

func info(version string, settings ...debug.BuildSetting) ReadBuildInfo {
	return func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: version}, Settings: settings}, true
	}
}

func TestResolve(t *testing.T) {
	rev := debug.BuildSetting{Key: "vcs.revision", Value: "0123456789abcdef"}
	dirty := debug.BuildSetting{Key: "vcs.modified", Value: "true"}
	none := func() (*debug.BuildInfo, bool) { return nil, false }
	cases := []struct {
		name, stamped string
		read          ReadBuildInfo
		want          string
	}{
		{"stamp wins", "v1.2.3", info("v9"), "v1.2.3"},
		{"module version", Unset, info("v0.4.7"), "v0.4.7"},
		{"vcs revision", Unset, info("(devel)", rev), "devel+0123456789ab"},
		{"dirty revision", Unset, info("(devel)", rev, dirty), "devel+0123456789ab-dirty"},
		{"nothing known", Unset, info("(devel)"), Unset},
		{"no build info", Unset, none, Unset},
	}
	for _, c := range cases {
		if got := Resolve(c.stamped, c.read); got != c.want {
			t.Errorf("%s: Resolve = %q, want %q", c.name, got, c.want)
		}
	}
}
