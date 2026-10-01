package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// Compute shipped in v1.1.0-rc.1, so the gap exists below 1.1.0 only.
func TestAPI072_GapExistsOnlyBelowTheFirstComputeRelease(t *testing.T) {
	cases := []struct {
		pin     string
		wantGap bool
	}{
		{"v1.0.0", true},
		{"v1.1.0-rc.1", false},
		{"v1.1.0", false},
	}
	for _, c := range cases {
		t.Run(c.pin, func(t *testing.T) {
			res := review.GoSourceAt("run.go", computeSrc, c.pin)
			if got := countRule(res, "API-072") > 0; got != c.wantGap {
				t.Fatalf("API-072 at %s = %v, want %v: %+v", c.pin, got, c.wantGap, res.Findings)
			}
		})
	}
}

func TestAPI064_AdmittedFromTheFirstComputeRelease(t *testing.T) {
	src := readFixture(t, "api_064_bad.go")
	for pin, want := range map[string]int{"v1.0.0": 0, "v1.1.0-rc.1": 1, "v1.1.0": 1} {
		if got := countRule(review.GoSourceAt("bad.go", src, pin), "API-064"); got != want {
			t.Errorf("API-064 at %s = %d findings, want %d", pin, got, want)
		}
	}
}

// API-070 and API-071 need only Group and Task, so a 1.0.0 pin keeps them.
func TestAPI070_071_FireAtV1_0_0(t *testing.T) {
	for _, id := range []string{"API-070", "API-071"} {
		badName, _ := compositionFixtures(id)
		res := review.GoSourceAt(badName, readFixture(t, badName), "v1.0.0")
		if countRule(res, id) == 0 {
			t.Errorf("%s did not fire at v1.0.0: %+v", id, res.Findings)
		}
	}
}
