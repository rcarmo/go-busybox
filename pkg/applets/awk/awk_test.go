package awk_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/awk"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestAwk(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "print",
			Args:     []string{"{print}", "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "a b c\n",
			},
			WantOut: "a b c\n",
		},
		{
			Name:     "print_field",
			Args:     []string{"{print $2}", "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "a b c\n",
			},
			WantOut: "b\n",
		},
		{
			Name:     "field_sep",
			Args:     []string{"-F", ",", "{print $2}", "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "a,b,c\n",
			},
			WantOut: "b\n",
		},
		{
			Name:     "variable",
			Args:     []string{"-v", "x=2", "{print $x}", "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "a b c\n",
			},
			WantOut: "b\n",
		},
		{
			Name:     "warn_option",
			Args:     []string{"-W", "posix", "{print}", "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "a b c\n",
			},
			WantOut: "a b c\n",
			WantErr: "warning: option -W is ignored",
		},
		{
			Name:     "program_file",
			Args:     []string{"-f", "prog.awk", "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"prog.awk":  "{print $2}\n",
				"input.txt": "a b c\n",
			},
			WantOut: "b\n",
		},
		{
			Name:     "arg_assignment",
			Args:     []string{"{print $x}", "x=2", "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "a b c\n",
			},
			WantOut: "b\n",
		},
		{
			Name:     "print_literal_and_field",
			Args:     []string{`{print "hello", $2}`, "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "a b c\n",
			},
			WantOut: "hello b\n",
		},
		{
			Name:     "print_var",
			Args:     []string{`{print x}`, "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "a b c\n",
			},
			Setup: func(t *testing.T, dir string) {
				_ = dir
			},
			WantOut: "\n",
		},
		{
			Name:     "ofs",
			Args:     []string{"-v", "OFS=:", `{print "a", "b", $2}`, "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "x y z\n",
			},
			WantOut: "a:b:y\n",
		},
		{
			Name:     "begin_end",
			Args:     []string{"BEGIN {print \"start\"} {print $2} END {print \"done\"}", "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "a b c\n",
			},
			WantOut: "start\nb\ndone\n",
		},
		{
			Name:     "regex_rule",
			Args:     []string{"/b/ {print $1}", "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "a b c\nx y z\n",
			},
			WantOut: "a\n",
		},
		{
			Name:     "assignment_expr",
			Args:     []string{"{x=$2+1; print x}", "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "1 2 3\n",
			},
			WantOut: "3\n",
		},
		{
			Name:     "assignment_chain",
			Args:     []string{"{x=1; y=x+2; print y}", "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "1 2 3\n",
			},
			WantOut: "3\n",
		},
		{
			Name:     "expr_parens",
			Args:     []string{"{print ($2+3)*2}", "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "1 2 3\n",
			},
			WantOut: "10\n",
		},
		{
			Name:     "string_literal",
			Args:     []string{`{print "a", "b", "c"}`, "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "1 2 3\n",
			},
			WantOut: "a b c\n",
		},
		{
			Name:     "predicate_compare",
			Args:     []string{"$2>2 {print $1}", "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "1 2 3\n4 5 6\n",
			},
			WantOut: "4\n",
		},
		{
			Name:     "predicate_regex_expr",
			Args:     []string{`$1~"a" {print $2}`, "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "a 1\nb 2\n",
			},
			WantOut: "1\n",
		},
		{
			Name:     "logical_ops",
			Args:     []string{`$1=="a" && $2>1 {print $2}`, "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "a 1\na 3\nb 4\n",
			},
			WantOut: "3\n",
		},
		{
			Name:     "if_else",
			Args:     []string{"{if ($2>2) {print $1} else {print \"no\"}}", "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "a 1\nb 3\n",
			},
			WantOut: "no\nb\n",
		},
		{
			Name:     "while_loop",
			Args:     []string{"BEGIN {i=0; while (i<3) {i=i+1; print i}}", "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "a 1\n",
			},
			WantOut: "1\n2\n3\n",
		},
		{
			Name:     "builtin_length",
			Args:     []string{`{print length($1)}`, "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "abcd ef\n",
			},
			WantOut: "4\n",
		},
		{
			Name:     "builtin_substr",
			Args:     []string{`{print substr($1,2,2)}`, "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "abcd ef\n",
			},
			WantOut: "bc\n",
		},
		{
			Name:     "builtin_toupper",
			Args:     []string{`{print toupper($1)}`, "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "abcd ef\n",
			},
			WantOut: "ABCD\n",
		},
		{
			Name:     "array_assign",
			Args:     []string{`BEGIN {a["x"]=2; print a["x"]}`},
			WantCode: core.ExitSuccess,
			WantOut:  "2\n",
		},
		{
			Name:     "for_loop",
			Args:     []string{`BEGIN {sum=0; for (i=0; i<3; i=i+1) {sum=sum+i}; print sum}`},
			WantCode: core.ExitSuccess,
			WantOut:  "3\n",
		},
		{
			Name:     "next_statement",
			Args:     []string{`$1=="skip" {next} {print $1}`, "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "skip a\nok b\n",
			},
			WantOut: "ok\n",
		},
		{
			Name:     "break_statement",
			Args:     []string{`BEGIN {i=0; while (1) {i=i+1; if (i==2) {break}; print i}}`},
			WantCode: core.ExitSuccess,
			WantOut:  "1\n",
		},
		{
			Name:     "continue_statement",
			Args:     []string{`BEGIN {i=0; while (i<3) {i=i+1; if (i==2) {continue}; print i}}`},
			WantCode: core.ExitSuccess,
			WantOut:  "1\n3\n",
		},
		{
			Name:     "nr_nf_vars",
			Args:     []string{`{print NR, NF}`, "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "a b\nc d e\n",
			},
			WantOut: "1 2\n2 3\n",
		},
	}
	testutil.RunAppletTests(t, awk.Run, tests)
}
