package main

import (
	"fmt"
	"sort"
	"strings"
)

// scoreEntries runs every bound test once per package and scores each entry.
// An entry passes only when every listed test reported pass.
func scoreEntries(entries []Entry, runner TestRunner, root, tags string) Score {
	outcomes := runPackages(entries, runner, root, tags)
	score := Score{
		IDs:      map[string]Status{},
		Sections: map[string]SectionTotals{},
		Tiers:    map[Tier]TierTotals{},
	}
	for _, e := range entries {
		status := entryStatus(e, outcomes)
		score.IDs[e.ID] = status
		tally(&score, e, status)
	}
	return score
}

func runPackages(entries []Entry, runner TestRunner, root, tags string) map[string]map[string]string {
	byPkg := map[string][]string{}
	for _, e := range entries {
		if e.Waiver != "" {
			continue
		}
		for _, t := range e.Tests {
			byPkg[t.Pkg] = append(byPkg[t.Pkg], t.Run)
		}
	}
	outcomes := map[string]map[string]string{}
	for pkg, names := range byPkg {
		names = uniqueSorted(names)
		stream, err := runner.Run(root, pkg, tags, names)
		if err != nil {
			outcomes[pkg] = map[string]string{}
			continue
		}
		outcomes[pkg] = parseOutcomes(stream)
	}
	return outcomes
}

func entryStatus(e Entry, outcomes map[string]map[string]string) Status {
	if strings.TrimSpace(e.Waiver) != "" {
		return StatusWaived
	}
	if len(e.Tests) == 0 {
		return StatusFail
	}
	for _, t := range e.Tests {
		if outcomes[t.Pkg][t.Run] != actionPass {
			return StatusFail
		}
	}
	return StatusPass
}

func tally(score *Score, e Entry, status Status) {
	tier := score.Tiers[e.Tier]
	tier.Total++
	section := score.Sections[e.Section]
	section.Total++
	switch status {
	case StatusPass:
		tier.Pass++
		section.Pass++
	case StatusWaived:
		tier.Waived++
	default:
		tier.Fail++
	}
	score.Tiers[e.Tier] = tier
	score.Sections[e.Section] = section
}

func uniqueSorted(in []string) []string {
	set := map[string]bool{}
	for _, s := range in {
		set[s] = true
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// summaryLines prints one line per tier in a stable order.
func summaryLines(score Score) []string {
	var lines []string
	for _, tier := range []Tier{Tier11, Tier12} {
		t := score.Tiers[tier]
		lines = append(lines, fmt.Sprintf("tier %s: %d pass, %d fail, %d waived of %d", tier, t.Pass, t.Fail, t.Waived, t.Total))
	}
	return lines
}
