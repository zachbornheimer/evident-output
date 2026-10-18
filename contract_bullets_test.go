package evo_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// maxBulletOverlap is the largest share of distinct words two bullets in one
// CONTRACT.md section may have in common (Jaccard: shared words over all
// words either uses). Two bullets that agree on 6 of every 10 distinct words
// state one rule twice, and when one is later edited the other contradicts it.
// The real bullets peak well under 0.4; the old and new wording of the §8
// After-hold rule scored above 0.75.
const maxBulletOverlap = 0.6

// shortBulletWords is the size up to which a bullet is a short rule. Adding
// words to a short rule drops its Jaccard overlap with the original below
// maxBulletOverlap, so a short bullet is judged by containment instead: the
// share of its own words that the other bullet repeats.
const shortBulletWords = 10

// maxShortContainment is the largest share of a short bullet's words another
// bullet in the section may repeat. No two real bullets come near it.
const maxShortContainment = 0.9

var contractSectionHeading = regexp.MustCompile(`^## `)

var contractWord = regexp.MustCompile("[a-z0-9_.`]+")

// contractBullet is one "- " item of a CONTRACT.md section, continuation
// lines joined.
type contractBullet struct {
	line  int
	words map[string]bool
}

// contractBullets returns the bullets of each CONTRACT.md section, keyed by the
// section's heading. Fenced code is not prose and is skipped.
func contractBullets(t *testing.T, path string) map[string][]contractBullet {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return parseContractBullets(string(body))
}

func parseContractBullets(body string) map[string][]contractBullet {
	sections := map[string][]contractBullet{}
	var heading string
	var texts []string
	var lines []int
	fenced := false
	flush := func() {
		for i, text := range texts {
			sections[heading] = append(sections[heading], contractBullet{lines[i], contractWords(text)})
		}
		texts, lines = nil, nil
	}
	for n, line := range strings.Split(body, "\n") {
		switch {
		case strings.HasPrefix(line, "```"):
			fenced = !fenced
		case fenced:
		case contractSectionHeading.MatchString(line):
			flush()
			heading = line
		case strings.HasPrefix(line, "- "):
			texts = append(texts, line[2:])
			lines = append(lines, n+1)
		case strings.HasPrefix(line, " ") && len(texts) > 0:
			texts[len(texts)-1] += " " + strings.TrimSpace(line)
		}
	}
	flush()
	return sections
}

func contractWords(text string) map[string]bool {
	words := map[string]bool{}
	for _, w := range contractWord.FindAllString(strings.ToLower(text), -1) {
		words[strings.Trim(w, ".")] = true
	}
	return words
}

func sharedWords(a, b contractBullet) int {
	shared := 0
	for w := range a.words {
		if b.words[w] {
			shared++
		}
	}
	return shared
}

// contractOverlap is the Jaccard overlap of two bullets' distinct words.
func contractOverlap(a, b contractBullet) float64 {
	return float64(sharedWords(a, b)) / float64(len(a.words)+len(b.words)-sharedWords(a, b))
}

// contractContainment is the share of the shorter bullet's distinct words that
// the other bullet repeats.
func contractContainment(a, b contractBullet) float64 {
	return float64(sharedWords(a, b)) / float64(min(len(a.words), len(b.words)))
}

// restatesRule reports whether two bullets state one rule twice: they overlap
// past maxBulletOverlap, or the shorter one is a short rule the other repeats.
func restatesRule(a, b contractBullet) bool {
	if contractOverlap(a, b) > maxBulletOverlap {
		return true
	}
	return min(len(a.words), len(b.words)) <= shortBulletWords && contractContainment(a, b) > maxShortContainment
}

// TestContractBulletsStateEachRuleOnce fails when two bullets in one CONTRACT.md
// section share more than maxBulletOverlap of their words, or one restates a
// short rule in full (restatesRule): a rule written
// twice drifts into two rules that contradict each other (the §8 After-hold
// bullet once did). Merge the pair into one bullet.
func TestContractBulletsStateEachRuleOnce(t *testing.T) {
	for heading, bullets := range contractBullets(t, "CONTRACT.md") {
		for i, a := range bullets {
			for _, b := range bullets[i+1:] {
				if restatesRule(a, b) {
					t.Errorf("CONTRACT.md lines %d and %d (%s) state one rule twice (%.0f%% of their words shared, %.0f%% of the shorter bullet repeated); state the rule once",
						a.line, b.line, heading, contractOverlap(a, b)*100, contractContainment(a, b)*100)
				}
			}
		}
	}
}

// TestContractBulletOverlapFlagsARestatedRule proves the guard can fail: the
// pre-fix §8 text, with both After-hold bullets, is over the limit.
func TestContractBulletOverlapFlagsARestatedRule(t *testing.T) {
	restated := "## 8. X\n\n" +
		"- Plain streams a Group or Sequence that is still open 500 ms after its first member settled as a block: its header row, then each member as it settles. A root row that ran After a Task or Group whose block is not yet written or closed waits for it, so a dependent never prints above the block that explains it; a row with no such predecessor never waits.\n" +
		"- Plain streams a Group or Sequence that is still open 500 ms after its first member settled as a block: its header row, then each member as it settles. A root row that ran After a Task or Group waits until the block holding it has closed, so a dependent never prints above the block that explains it; a row with no such predecessor never waits.\n"
	bullets := parseContractBullets(restated)["## 8. X"]
	if len(bullets) != 2 {
		t.Fatalf("parsed %d bullets, want 2", len(bullets))
	}
	if o := contractOverlap(bullets[0], bullets[1]); o <= maxBulletOverlap {
		t.Fatalf("restated rule scores %.2f, not above %.2f", o, maxBulletOverlap)
	}
}

// TestContractBulletGuardFlagsARestatedShortRule proves the guard fails on a
// short rule restated beside its original: the longer wording scores under
// maxBulletOverlap on Jaccard alone, but repeats every word of the short one.
func TestContractBulletGuardFlagsARestatedShortRule(t *testing.T) {
	restated := "## 8. X\n\n" +
		"- Cancellation preserves committed truth and never implies rollback.\n" +
		"- Cancellation preserves committed truth and never implies rollback of any committed change, whatever the output.\n"
	bullets := parseContractBullets(restated)["## 8. X"]
	if len(bullets) != 2 {
		t.Fatalf("parsed %d bullets, want 2", len(bullets))
	}
	if o := contractOverlap(bullets[0], bullets[1]); o > maxBulletOverlap {
		t.Fatalf("fixture scores %.2f on Jaccard, so it no longer proves the containment path", o)
	}
	if !restatesRule(bullets[0], bullets[1]) {
		t.Fatal("a short rule restated in a longer bullet was not flagged")
	}
}
