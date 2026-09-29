// Package rules is the stable review-rule registry (Appendix C namespaces).
package rules

// Rule is one stable diagnostic rule (MCP-027 / §28.4.5 / Appendix C).
type Rule struct {
	ID              string   `json:"id"`
	Category        string   `json:"category"`
	Severity        Severity `json:"severity"`
	Invariant       string   `json:"invariant"`
	Why             string   `json:"why"`
	BadCode         string   `json:"bad_code"`
	GoodCode        string   `json:"good_code"`
	BadOutput       string   `json:"bad_output,omitempty"`
	GoodOutput      string   `json:"good_output,omitempty"`
	Remediation     string   `json:"remediation"`
	Exceptions      []string `json:"exceptions,omitempty"`
	RelatedGuidance []string `json:"related_guidance,omitempty"`
	VerificationIDs []string `json:"verification_ids,omitempty"`
	Since           string   `json:"since"` // first version (MCP-028)
	// MinDialect is the oldest evident-output release whose API can apply
	// this rule's GoodCode and Remediation; empty means any. Review never
	// reports the rule for an older desired_version, and every finding of
	// it carries this as RequiredVersion. It is stated here once, never on
	// a detector.
	MinDialect  string    `json:"min_dialect,omitempty"`
	Deprecated  bool      `json:"deprecated"`
	Replacement string    `json:"replacement,omitempty"`
	Certainty   Certainty `json:"certainty,omitempty"`
	Detection   Detection `json:"detection,omitempty"`
}

// familyRegistry collects rule slices contributed by sibling files in this
// package (rules_ui.go, rules_wire.go, rules_file.go, rules_provenance.go,
// ...). A family file registers itself from its own init() via
// registerFamily, so adding a family never requires editing All() itself —
// parallel family additions land in their own files and merge without
// touching the same lines.
var familyRegistry [][]Rule

// registerFamily adds one family's rules to the v1 registry. Call this from
// a new rules_<family>.go file's init(); never edit All() to add a family.
func registerFamily(rs []Rule) {
	familyRegistry = append(familyRegistry, rs)
}

// All returns the v1 rule registry: every family registered via
// registerFamily. IDs and meanings obey version policy: IDs never rename;
// deprecations dual-write via Deprecated+Replacement (MCP-028).
func All() []Rule {
	var all []Rule
	for _, family := range familyRegistry {
		all = append(all, family...)
	}
	return all
}

// Explain returns a rule by ID (MCP-027 full payload).
func Explain(id string) (Rule, bool) {
	for _, r := range All() {
		if r.ID == id {
			return r, true
		}
	}
	return Rule{}, false
}

// IDs returns the stable set of rule identifiers (MCP-028 compatibility surface).
func IDs() []string {
	all := All()
	out := make([]string, len(all))
	for i, r := range all {
		out[i] = r.ID
	}
	return out
}
