package project

import "github.com/zachbornheimer/evident-output/v2/internal/record"

// ContainerMembers is how many direct members (child Tasks and child
// collections) each container holds, by container ID. The ledger fold
// compares it with how many of those members contributed to a row, because
// a row may name a container only when every member of it did the work. A
// container missing from the map is one the snapshot does not describe (a
// live frame left it out), and the fold then asks nothing of it.
type ContainerMembers map[string]int

// MembersOf is the direct member count of every container in snap.
func MembersOf(snap record.Snapshot) ContainerMembers {
	members := ContainerMembers{}
	for _, col := range snap.Collections {
		members.add(col)
	}
	return members
}

// add records col and the containers nested in it.
func (m ContainerMembers) add(col record.TasksSnapshot) {
	m[col.ID] = directTaskCount(col) + len(col.Collections) + CollectionTallyOf(col).Count
	for _, nested := range col.Collections {
		m.add(nested)
	}
}

// directTaskCount is every child Task of col, including those a live
// projection left out of Tasks.
func directTaskCount(col record.TasksSnapshot) int {
	if tally, partial := ChildTallyOf(col); partial {
		return tally.All.Total
	}
	return len(col.Tasks)
}
