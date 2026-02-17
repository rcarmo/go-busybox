package sed_test

import (
	"strings"
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/sed"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// isSafeExpr returns true if the expression is unlikely to cause
// catastrophic backtracking or infinite loops during fuzzing.
func isSafeExpr(expr string) bool {
	// Reject expressions with too many quantifiers (backtracking risk)
	if strings.Count(expr, "*") > 2 || strings.Count(expr, "+") > 2 {
		return false
	}
	// Reject very long expressions
	if len(expr) > 80 {
		return false
	}
	// Reject expressions with nested groups and quantifiers
	if strings.Contains(expr, "\\(") && (strings.Contains(expr, "*") || strings.Contains(expr, "+")) {
		return false
	}
	// Reject branch/label commands that can create infinite loops
	if strings.Contains(expr, "b ") || strings.Contains(expr, "b\t") ||
		strings.Contains(expr, "; b") || strings.Contains(expr, ";b") {
		return false
	}
	// Reject N;P;D and N;D patterns that can loop
	if strings.Contains(expr, "N") && strings.Contains(expr, "D") {
		return false
	}
	return true
}

// FuzzSed fuzzes sed with fuzzed input data and a basic substitution command.
func FuzzSed(f *testing.F) {
	f.Add([]byte("sample input"))
	f.Add([]byte(""))
	f.Add([]byte("aaa\nbbb\nccc\n"))
	f.Add([]byte("line without newline"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"s/a/b/", "input.txt"}
		files := map[string]string{
			"input.txt": input,
		}
		testutil.FuzzCompare(t, "sed", sed.Run, args, input, files, testutil.FuzzOptions{SharedDir: true})
	})
}

// FuzzSedExpr fuzzes the sed expression itself, testing the parser
// against arbitrary byte sequences to ensure it never panics.
func FuzzSedExpr(f *testing.F) {
	// Seed with valid sed expressions
	f.Add("s/a/b/")
	f.Add("s/a/b/g")
	f.Add("s/a/b/p")
	f.Add("s/a/b/gp")
	f.Add("/pattern/d")
	f.Add("/pattern/p")
	f.Add("y/abc/xyz/")
	f.Add("1,5d")
	f.Add("1~2d")
	f.Add("/start/,/end/d")
	f.Add("a\\ appended text")
	f.Add("i\\ inserted text")
	f.Add("c\\ changed text")
	f.Add("{s/a/b/;s/c/d/}")
	f.Add("h;g;x")
	f.Add("H;G")
	f.Add("=")
	f.Add("q")
	f.Add("")
	// Seed with edge cases
	f.Add("s///")
	f.Add("s/[a-z]/X/g")
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, expr string) {
		if !isSafeExpr(expr) {
			return
		}
		input := "hello world\nfoo bar\nbaz qux\n"
		args := []string{expr}
		testutil.FuzzCompare(t, "sed", sed.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzSedMultiCmd fuzzes sed with multiple -e expressions to test
// command chaining and interaction between commands.
func FuzzSedMultiCmd(f *testing.F) {
	f.Add("hello\nworld\nfoo\nbar\n", "s/o/0/g", "s/l/L/g")
	f.Add("abc\ndef\n", "/abc/d", "s/d/D/")
	f.Add("line1\nline2\nline3\n", "2d", "s/line/LINE/")
	f.Add("aaa\nbbb\nccc\n", "1,2s/a/x/g", "3s/c/y/g")
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, input, expr1, expr2 string) {
		input = testutil.ClampString(input, 512)
		if !isSafeExpr(expr1) || !isSafeExpr(expr2) {
			return
		}
		args := []string{"-e", expr1, "-e", expr2}
		testutil.FuzzCompare(t, "sed", sed.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzSedFlags fuzzes sed with various flag combinations and fixed input
// to test flag parsing.
func FuzzSedFlags(f *testing.F) {
	f.Add([]byte("hello\nworld\n"))
	f.Add([]byte("abc\nabc\nabc\n"))
	f.Add([]byte(""))
	f.Add([]byte("single line"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	flagSets := [][]string{
		{"-n", "p"},
		{"-n", "/hello/p"},
		{"s/a/b/g"},
		{"-n", "s/a/b/gp"},
		{"2d"},
		{"1,2d"},
		{"-e", "s/a/b/", "-e", "s/c/d/"},
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		for _, args := range flagSets {
			testutil.FuzzCompare(t, "sed", sed.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
		}
	})
}
