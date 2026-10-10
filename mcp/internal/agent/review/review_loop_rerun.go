package review

import (
	"go/ast"
	"go/token"
)

// loopChain is the loops around one call, outermost first.
type loopChain []*loopScope

// noLoop is a transfer target outside every loop in the chain.
const noLoop = -1

// reruns reports whether some loop in c runs call again on the value recv
// names without rebinding it. Walking out from the innermost loop, a loop
// reruns the call when the call's block falls through to its next
// iteration. When the block transfers control instead, return leaves every
// loop; break leaves loops and hands the question to the loop control
// resumes in; continue L, or goto into an enclosing loop, reruns the call
// in that loop. A loop left or resumed that binds recv makes it a fresh
// value each time.
func (c loopChain) reruns(call ast.Node, recv string, labels map[string]nodeSpan) bool {
	for i := len(c) - 1; i >= 0; {
		if c[i].bound[recv] {
			return false
		}
		end, ok := c[i].endAround(call)
		if !ok {
			return true
		}
		target, rerun := c.transfer(i, end, labels)
		if rerun && target == i {
			return true
		}
		if c.binds(target+1, i, recv) {
			return false
		}
		if rerun {
			return !c[target].bound[recv]
		}
		i = target
	}
	return false
}

// transfer is where end sends control from loop i: the index of the loop
// it lands in (noLoop for none) and whether it lands in that loop's next
// run of the call (rerun) or after the loops it left.
func (c loopChain) transfer(i int, end blockEnd, labels map[string]nodeSpan) (target int, rerun bool) {
	branch, ok := end.last.(*ast.BranchStmt)
	if !ok {
		return noLoop, false // return
	}
	if branch.Label == nil {
		if branch.Tok == token.CONTINUE || end.inSwitch {
			return i, true
		}
		return i - 1, false
	}
	name := branch.Label.Name
	switch branch.Tok {
	case token.CONTINUE:
		if k := c.labeled(i, name); k != noLoop {
			return k, true
		}
		return i, true // not a loop label: invalid Go, read as staying
	case token.BREAK:
		target = c.holding(i, labels[name])
		return target, target == i
	default: // goto
		target = c.holding(i, labels[name])
		return target, target != noLoop
	}
}

// labeled is the index of the loop at or above i that carries label.
func (c loopChain) labeled(i int, label string) int {
	for ; i >= 0; i-- {
		if c[i].label == label {
			return i
		}
	}
	return noLoop
}

// holding is the innermost loop at or above i whose body holds span.
func (c loopChain) holding(i int, span nodeSpan) int {
	for ; i >= 0; i-- {
		if c[i].span.contains(span) {
			return i
		}
	}
	return noLoop
}

// binds reports whether any loop with index in [from, to) binds recv.
func (c loopChain) binds(from, to int, recv string) bool {
	for i := max(from, 0); i < to; i++ {
		if c[i].bound[recv] {
			return true
		}
	}
	return false
}
