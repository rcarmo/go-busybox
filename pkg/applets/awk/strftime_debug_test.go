package awk

import (
	"testing"
)

func TestStrftimeDebug(t *testing.T) {
	exprText := `strftime("%s", 0)`
	expr, err := parseExpr(exprText)
	if err != nil {
		t.Fatalf("parseExpr failed: %v", err)
	}
	if expr == nil {
		t.Fatalf("expr nil")
	}
	if expr.kind != exprFunc {
		t.Fatalf("expected func expr, got kind=%v", expr.kind)
	}
	t.Logf("expr.name=%q nargs=%d", expr.name, len(expr.args))
	for i, a := range expr.args {
		t.Logf("arg[%d] kind=%v value=%q num=%v redir=%q", i, a.kind, a.value, a.num, a.redir)
	}
	state := &awkState{vars: map[string]string{}, arrays: map[string]map[string]string{}, fs: " ", ofs: " "}
	val1, num1 := evalExpr(expr.args[0], state)
	val2, num2 := evalExpr(expr.args[1], state)
	t.Logf("arg evals: 0->%q %v, 1->%q %v", val1, num1, val2, num2)
	// inspect convertStrftime
	t.Logf("convertStrftime(%%s)=%q", convertStrftime(val1))
	val, num := evalExpr(expr, state)
	t.Logf("evalExpr returned val=%q num=%v", val, num)
}
