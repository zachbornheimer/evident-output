package engine

import (
	"github.com/zachbornheimer/evident-output/internal/ordered"
)

// ledgerIndex finds the run's ledger sections by owner and by the name they
// are shown under, in O(1). Guarded by o.mu.
type ledgerIndex struct {
	byOwner map[ledgerSectionKey]*ledgerSection
	byName  map[string][]*ledgerSection
}

// opened indexes a newly opened section.
func (x *ledgerIndex) opened(key ledgerSectionKey, s *ledgerSection) {
	if x.byOwner == nil {
		x.byOwner = map[ledgerSectionKey]*ledgerSection{}
		x.byName = map[string][]*ledgerSection{}
	}
	x.byOwner[key] = s
	x.byName[s.owner.name] = append(x.byName[s.owner.name], s)
}

// insertByLedgerOrder places section after every section whose order is
// less than or equal to its own, keeping ties in arrival order.
func insertByLedgerOrder(sections []*ledgerSection, section *ledgerSection) []*ledgerSection {
	return ordered.Insert(sections, section, (*ledgerSection).order)
}
