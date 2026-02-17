package awk_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/awk"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzAwk fuzzes the awk program string to test the parser and interpreter
// against arbitrary programs with fixed input.
func FuzzAwk(f *testing.F) {
	// Valid awk programs
	f.Add([]byte("print"))
	f.Add([]byte("{print $1}"))
	f.Add([]byte("{x=$2+1; print x}"))
	f.Add([]byte("BEGIN {print \"start\"} {print $2} END {print \"end\"}"))
	f.Add([]byte("/b/ {print $1}"))
	f.Add([]byte("{print \"a\", \"b\", $2}"))
	f.Add([]byte("$2>2 {print $1}"))
	f.Add([]byte("$1==\"a\" && $2>1 {print $2}"))
	f.Add([]byte("$1~\"a\" {print $2}"))
	f.Add([]byte("{if ($2>1) {print $1} else {print \"no\"}}"))
	f.Add([]byte("BEGIN {i=0; while (i<2) {i=i+1; print i}}"))
	f.Add([]byte("{print length($1)}"))
	f.Add([]byte("{print substr($1,2,2)}"))
	f.Add([]byte("{print toupper($1)}"))
	f.Add([]byte("{print NR, NF, $0}"))
	f.Add([]byte("{gsub(/a/,\"x\"); print}"))
	f.Add([]byte("BEGIN{FS=\":\"}{print $1}"))
	f.Add([]byte("{printf \"%s\\n\", $1}"))
	f.Add([]byte("{a[$1]++} END{for(k in a) print k, a[k]}"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		prog := testutil.ClampString(string(data), 64)
		if prog == "" {
			prog = "print"
		}
		files := map[string]string{"input.txt": "a b c\n"}
		args := []string{prog, "input.txt"}
		testutil.FuzzCompare(t, "awk", awk.Run, args, "", files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzAwkInput fuzzes the input data with various fixed awk programs
// to test the input processing paths.
func FuzzAwkInput(f *testing.F) {
	f.Add([]byte("hello world\n"))
	f.Add([]byte("a b c\nd e f\n"))
	f.Add([]byte(""))
	f.Add([]byte("1 2 3\n4 5 6\n7 8 9\n"))
	f.Add([]byte("no:fields:here\n"))
	f.Add([]byte("  leading spaces\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	programs := []string{
		"{print $1}",
		"{print NF}",
		"{print NR, $0}",
		"$1 > \"m\" {print}",
		"{print length($0)}",
		"/^[0-9]/{print $0}",
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		for _, prog := range programs {
			args := []string{prog}
			testutil.FuzzCompare(t, "awk", awk.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
		}
	})
}
