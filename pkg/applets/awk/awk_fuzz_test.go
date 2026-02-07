package awk_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/awk"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzAwk(f *testing.F) {
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
