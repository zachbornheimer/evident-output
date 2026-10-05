package main

import "testing"

func TestResolveTestTarget(t *testing.T) {
	fsys := newFakeFS(map[string]string{
		"/repo/go.mod":               "module root",
		"/repo/eval/go.mod":          "module eval",
		"/repo/eval/runner/run.go":   "package runner",
		"/repo/internal/engine/e.go": "package engine",
	})
	cases := []struct {
		name, root, pkg string
		want            testTarget
	}{
		{"nested module package runs from the module", "/repo", "./eval/runner", testTarget{Dir: "/repo/eval", Pkg: "./runner"}},
		{"module root package", "/repo", "./eval", testTarget{Dir: "/repo/eval", Pkg: "."}},
		{"root module package runs from root", "/repo", "./internal/engine", testTarget{Dir: "/repo", Pkg: "./internal/engine"}},
		{"root package runs from root", "/repo", ".", testTarget{Dir: "/repo", Pkg: "."}},
		{"unclean root is normalized", "/repo/", "./eval/runner", testTarget{Dir: "/repo/eval", Pkg: "./runner"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveTestTarget(fsys, tc.root, tc.pkg); got != tc.want {
				t.Errorf("resolveTestTarget(%q, %q) = %+v, want %+v", tc.root, tc.pkg, got, tc.want)
			}
		})
	}
}

func TestResolveTestTargetWithRelativeRoot(t *testing.T) {
	fsys := newFakeFS(map[string]string{"eval/go.mod": "module eval"})
	got := resolveTestTarget(fsys, ".", "./eval/driver")
	if want := (testTarget{Dir: "eval", Pkg: "./driver"}); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
