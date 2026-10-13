package gitenv

import (
	"slices"
	"testing"
)

func TestScrubDropsEveryRepositoryLocationVariable(t *testing.T) {
	env := []string{"PATH=/bin", "HOME=/h"}
	for name := range repositoryLocationVars {
		env = append(env, name+"=/hostile")
	}
	got := Scrub(env)
	if want := []string{"PATH=/bin", "HOME=/h"}; !slices.Equal(got, want) {
		t.Fatalf("Scrub = %v, want %v", got, want)
	}
}

func TestScrubKeepsSimilarlyNamedVariables(t *testing.T) {
	env := []string{"GIT_DIRECTORY=x", "GIT_AUTHOR_NAME=n", "MY_GIT_DIR=y"}
	if got := Scrub(env); !slices.Equal(got, env) {
		t.Fatalf("Scrub = %v, want %v unchanged", got, env)
	}
}

func TestScrubDoesNotMutateInput(t *testing.T) {
	env := []string{"GIT_DIR=/a", "PATH=/bin"}
	Scrub(env)
	if env[0] != "GIT_DIR=/a" {
		t.Fatalf("Scrub mutated its input: %v", env)
	}
}
