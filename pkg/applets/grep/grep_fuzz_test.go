package grep_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/grep"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzGrep fuzzes grep input with a fixed pattern, comparing against
// the reference busybox.
func FuzzGrep(f *testing.F) {
	f.Add([]byte("match"))
	f.Add([]byte("no match here"))
	f.Add([]byte("line1\nmatch\nline3\n"))
	f.Add([]byte(""))
	f.Add([]byte("MATCH\nMatch\nmatch"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"match"}
		testutil.FuzzCompare(t, "grep", grep.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzGrepPattern fuzzes the grep pattern itself to test the regex
// parser robustness. Input is fixed.
func FuzzGrepPattern(f *testing.F) {
	f.Add("hello")
	f.Add("^start")
	f.Add("end$")
	f.Add("a.*b")
	f.Add("[a-z]+")
	f.Add("\\bword\\b")
	f.Add("(group)")
	f.Add("")
	f.Add(".")
	f.Add("\\")
	f.Add("[")
	f.Add("*")
	f.Add("foo|bar")
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, pattern string) {
		pattern = testutil.ClampString(pattern, 256)
		input := "hello world\nfoo bar\nbaz qux\nHELLO WORLD\n123 456\n"
		args := []string{pattern}
		testutil.FuzzCompare(t, "grep", grep.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzGrepFlags tests grep with various flag combinations and fuzzed input.
func FuzzGrepFlags(f *testing.F) {
	f.Add([]byte("hello\nworld\nHELLO\nWORLD\n"))
	f.Add([]byte("foo\nfoo\nbar\n"))
	f.Add([]byte(""))
	f.Add([]byte("single line with hello in it"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	flagSets := [][]string{
		{"-i", "hello"},
		{"-v", "hello"},
		{"-c", "hello"},
		{"-n", "hello"},
		{"-o", "hello"},
		{"-i", "-c", "hello"},
		{"-i", "-v", "hello"},
		{"-x", "hello"},
		{"-w", "hello"},
		{"-E", "hel+o"},
		{"-F", "hel+o"},
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		for _, args := range flagSets {
			testutil.FuzzCompare(t, "grep", grep.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
		}
	})
}
