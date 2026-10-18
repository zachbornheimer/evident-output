package review

import (
	"slices"
	"strings"
	"unicode"
)

// irregularUnits are the units whose plural the regular suffix rule gets
// wrong, keyed by singular.
var irregularUnits = map[string]string{
	"child":            "children",
	"tooth":            "teeth",
	"foot":             "feet",
	"leaf":             "leaves",
	"index":            "indices",
	"package in .venv": "packages in .venv",
}

// singularsEndingInS end in "s" and are still singular ("class", "status").
var singularsEndingInS = []string{"ss", "us", "is"}

// pluralUnit is unit as 2.0 renders it beside a quantity of count: the
// singular when count is one, else the plural. It repeats the rule of
// Effect's ledger (internal/project Pluralize), which a module outside the
// root cannot import; TestCountRewriteMatchesEffectPluralization holds the
// two together.
func pluralUnit(count int64, unit string) string {
	if count == 1 || unit == "" {
		return unit
	}
	if plural, ok := irregularUnits[unit]; ok {
		return plural
	}
	if !isPlainWords(unit) || endsInPluralS(unit) {
		return unit
	}
	switch {
	case strings.HasSuffix(unit, "y") && !endsInVowelY(unit):
		return strings.TrimSuffix(unit, "y") + "ies"
	case strings.HasSuffix(unit, "s"), strings.HasSuffix(unit, "x"), strings.HasSuffix(unit, "z"),
		strings.HasSuffix(unit, "ch"), strings.HasSuffix(unit, "sh"):
		return unit + "es"
	}
	return unit + "s"
}

// isPlainWords is true for letters and spaces only: a glob, path or symbol
// unit renders unchanged at any count.
func isPlainWords(unit string) bool {
	return !strings.ContainsFunc(unit, func(r rune) bool { return !unicode.IsLetter(r) && r != ' ' })
}

// endsInPluralS is true when the last word of unit is already a plural.
func endsInPluralS(unit string) bool {
	for singular, plural := range irregularUnits {
		if unit == plural {
			return true
		}
		if unit == singular {
			return false
		}
	}
	head := unit[strings.LastIndex(unit, " ")+1:]
	if !strings.HasSuffix(head, "s") {
		return false
	}
	return !slices.ContainsFunc(singularsEndingInS, func(ending string) bool { return strings.HasSuffix(head, ending) })
}

func endsInVowelY(unit string) bool {
	return len(unit) >= 2 && strings.ContainsRune("aeiou", rune(unit[len(unit)-2]))
}
