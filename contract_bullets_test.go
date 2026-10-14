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

// minBulletWords keeps a short bullet from tripping the guard on a few shared
// connectives; a rule stated in fewer words cannot be restated at length.
const minBulletWords = 12

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

// contractOverlap is the Jaccard overlap of two bullets' distinct words.
func contractOverlap(a, b contractBullet) float64 {
	shared := 0
	for w := range a.words {
		if b.words[w] {
			shared++
		}
	}
	return float64(shared) / float64(len(a.words)+len(b.words)-shared)
}

// TestContractBulletsStateEachRuleOnce fails when two bullets in one CONTRACT.md
// section share more than maxBulletOverlap of their words: a rule written
// twice drifts into two rules that contradict each other (the §8 After-hold
// bullet once did). Merge the pair into one bullet.
func TestContractBulletsStateEachRuleOnce(t *testing.T) {
	for heading, bullets := range contractBullets(t, "CONTRACT.md") {
		for i, a := range bullets {
			for _, b := range bullets[i+1:] {
				if len(a.words) < minBulletWords || len(b.words) < minBulletWords {
					continue
				}
				if o := contractOverlap(a, b); o > maxBulletOverlap {
					t.Errorf("CONTRACT.md lines %d and %d (%s) share %.0f%% of their words (limit %.0f%%); state the rule once",
						a.line, b.line, heading, o*100, maxBulletOverlap*100)
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
