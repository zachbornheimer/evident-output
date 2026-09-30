package main

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

const (
	ratchetPath         = "conformance/spec/ratchet.json"
	contractTestsPrefix = "conformance/contract/"
	goldensPrefix       = "conformance/goldens/"
	buildTagLine        = "//go:build evopending"
)

var (
	frozenGlobs = []string{
		"conformance/spec/req/*.json",
		"testdata/api_golden*.txt",
		".evor/baseline.sha256",
	}
	ratchetIDPattern = regexp.MustCompile(`C\d+-\d{3}`)
)

func isFrozen(p string) bool {
	if strings.HasPrefix(p, goldensPrefix) {
		return true
	}
	for _, glob := range frozenGlobs {
		if ok, _ := path.Match(glob, p); ok {
			return true
		}
	}
	return false
}

// check returns one line per violation in `base...HEAD`; empty means clean.
func check(g Git, base string, allowIDs map[string]bool) ([]string, error) {
	changes, err := g.NameStatus(base)
	if err != nil {
		return nil, fmt.Errorf("list changes since %s: %w", base, err)
	}
	var out []string
	touchesRatchet := false
	for _, c := range changes {
		switch {
		case isFrozen(c.Path):
			out = append(out, fmt.Sprintf("%s: frozen file changed (%s)", c.Path, c.Status))
		case strings.HasPrefix(c.Path, contractTestsPrefix):
			v, err := checkContractTest(g, base, c)
			if err != nil {
				return nil, err
			}
			out = append(out, v...)
		case c.Path == ratchetPath:
			touchesRatchet = true
		}
	}
	if touchesRatchet {
		v, err := checkRatchet(g, base, allowIDs)
		if err != nil {
			return nil, err
		}
		out = append(out, v...)
	}
	return out, nil
}

// checkContractTest allows one edit: deleting the evopending build-tag line
// and the blank line after it.
func checkContractTest(g Git, base string, c Change) ([]string, error) {
	if c.Status != statusModified {
		return []string{fmt.Sprintf("%s: contract test may not be added, deleted, or moved (%s)", c.Path, c.Status)}, nil
	}
	diff, err := g.FileDiff(base, c.Path)
	if err != nil {
		return nil, err
	}
	added, removed := changedLines(diff)
	if len(added) == 0 && onlyBuildTagRemoved(removed) {
		return nil, nil
	}
	return []string{fmt.Sprintf("%s: contract test edited beyond deleting %q", c.Path, buildTagLine)}, nil
}

func onlyBuildTagRemoved(removed []string) bool {
	if len(removed) == 0 || removed[0] != buildTagLine {
		return false
	}
	return len(removed) == 1 || (len(removed) == 2 && removed[1] == "")
}

func checkRatchet(g Git, base string, allowIDs map[string]bool) ([]string, error) {
	diff, err := g.FileDiff(base, ratchetPath)
	if err != nil {
		return nil, err
	}
	added, removed := changedLines(diff)
	addedIDs, removedIDs := idSet(added), idSet(removed)
	var out []string
	for _, id := range sortedKeys(removedIDs) {
		if !addedIDs[id] {
			out = append(out, fmt.Sprintf("%s: ratchet ID removed", id))
		}
	}
	for _, id := range sortedKeys(addedIDs) {
		if !removedIDs[id] && !allowIDs[id] {
			out = append(out, fmt.Sprintf("%s: ratchet ID added but not in --allow-ids", id))
		}
	}
	return out, nil
}

// changedLines splits a unified diff into added and removed line bodies.
func changedLines(diff string) (added, removed []string) {
	for line := range strings.SplitSeq(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		case strings.HasPrefix(line, "+"):
			added = append(added, line[1:])
		case strings.HasPrefix(line, "-"):
			removed = append(removed, line[1:])
		}
	}
	return added, removed
}

func idSet(lines []string) map[string]bool {
	set := map[string]bool{}
	for _, l := range lines {
		for _, id := range ratchetIDPattern.FindAllString(l, -1) {
			set[id] = true
		}
	}
	return set
}
