package eval_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

const fakeCredential = "not-a-real-credential"

// runScript runs run.sh with only the given environment, so the developer's
// real credential can never leak into the test.
func runScript(t *testing.T, env []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{"run.sh"}, args...)...)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// run.sh refuses before building or spending anything.
func TestRunScript_RefusesWithoutCredentialMaxUSDOrConfirmation(t *testing.T) {
	cases := []struct {
		name string
		env  []string
		args []string
		want string
	}{
		{"no credential", nil, []string{"--max-usd", "1", "--confirm-spend", "--all-ready"}, "ANTHROPIC_API_KEY is not set"},
		{"no max-usd", []string{"ANTHROPIC_API_KEY=" + fakeCredential}, []string{"--confirm-spend", "--all-ready"}, "--max-usd is required"},
		{"no confirm-spend", []string{"ANTHROPIC_API_KEY=" + fakeCredential}, []string{"--max-usd", "1", "--all-ready"}, "--confirm-spend is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runScript(t, tc.env, tc.args...)
			if err == nil {
				t.Fatalf("script must exit non-zero; output:\n%s", out)
			}
			if !strings.Contains(out, "refusing to start") || !strings.Contains(out, tc.want) {
				t.Errorf("output lacks %q:\n%s", tc.want, out)
			}
		})
	}
}
