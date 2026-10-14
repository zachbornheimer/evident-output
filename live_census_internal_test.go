package evo

// walkCensus counts every Task at or below col the slow way.
func walkCensus(col *tasksState, c *liveCensus) {
	for _, st := range col.taskStates() {
		c.total++
		c.count(st.node.Rec.State(), 1)
		if len(st.node.Rec.Warnings()) > 0 {
			c.warned++
		}
		if st.unstampedIn(st.node.Rec.State()) {
			c.unstamped++
		}
		c.nameWidth = max(c.nameWidth, len([]rune(st.name)))
	}
	for _, child := range col.childStates() {
		walkCensus(child, c)
	}
}
