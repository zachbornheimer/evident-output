package review

import (
	"go/ast"
	"go/token"
)

// 1.x Exec returned only an error; 2.0 returns (ExecResult, error). A call
// whose single result a 1.x program kept does not compile.

// scanExecResults flags every evo.Exec whose lone result is returned from a
// func with one result or assigned to one name. A statement that discards
// both results, and a func that returns both, compile in 2.0 and stay silent.
func (d *removedTwoZeroDetector) scanExecResults() {
	ast.Inspect(d.file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			if n.Body != nil {
				d.scanExecIn(n.Type, n.Body)
			}
		case *ast.FuncLit:
			d.scanExecIn(n.Type, n.Body)
		}
		return true
	})
}

// scanExecIn scans body without entering the func literals inside it; the
// walk in scanExecResults reaches each of those with its own signature.
func (d *removedTwoZeroDetector) scanExecIn(sig *ast.FuncType, body *ast.BlockStmt) {
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			if sig.Results.NumFields() == 1 && len(n.Results) == 1 {
				d.reportExecReturn(n)
			}
		case *ast.AssignStmt:
			if len(n.Lhs) == 1 && len(n.Rhs) == 1 {
				d.reportExecAssign(n)
			}
		}
		return true
	})
}

const execResultMessage = "evo.Exec returns (ExecResult, error) in 2.0; this call keeps only the error"

func (d *removedTwoZeroDetector) reportExecReturn(ret *ast.ReturnStmt) {
	call, ok := d.isEvoCall(ret.Results[0], "Exec")
	if !ok {
		return
	}
	d.report(ret, execResultMessage, "replace "+d.nodeSrc(ret)+" with if _, err := "+d.nodeSrc(call)+"; err != nil { return err }; return nil")
}

func (d *removedTwoZeroDetector) reportExecAssign(as *ast.AssignStmt) {
	call, ok := d.isEvoCall(as.Rhs[0], "Exec")
	if !ok || (as.Tok != token.DEFINE && as.Tok != token.ASSIGN) {
		return
	}
	d.report(as, execResultMessage, "replace "+d.nodeSrc(as)+" with _, "+d.nodeSrc(as.Lhs[0])+" "+as.Tok.String()+" "+d.nodeSrc(call))
}
