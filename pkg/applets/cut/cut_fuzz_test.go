package cut_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/cut"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzCut fuzzes cut with comma-delimited field extraction and fuzzed input.
func FuzzCut(f *testing.F) {
	f.Add([]byte("a,b,c\n"))
	f.Add([]byte(""))
	f.Add([]byte("one,two,three\nfour,five,six\n"))
	f.Add([]byte("no delimiters here\n"))
	f.Add([]byte(",,,\n"))
	f.Add([]byte("a\tb\tc\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"-d", ",", "-f", "1"}
		testutil.FuzzCompare(t, "cut", cut.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzCutFields fuzzes cut field specifications.
func FuzzCutFields(f *testing.F) {
	f.Add("1")
	f.Add("1,2")
	f.Add("1-3")
	f.Add("2-")
	f.Add("-3")
	f.Add("1,3-5")
	f.Add("1-2,4-5")
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, fields string) {
		fields = testutil.ClampString(fields, 64)
		input := "a:b:c:d:e\n1:2:3:4:5\n"
		args := []string{"-d", ":", "-f", fields}
		testutil.FuzzCompare(t, "cut", cut.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzCutChars fuzzes cut with character selection mode.
func FuzzCutChars(f *testing.F) {
	f.Add([]byte("hello world\n"))
	f.Add([]byte("abcdefghij\n1234567890\n"))
	f.Add([]byte(""))
	f.Add([]byte("x\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"-c", "1-5"}
		testutil.FuzzCompare(t, "cut", cut.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzCutFlags tests cut with multiple delimiter/flag combinations.
func FuzzCutFlags(f *testing.F) {
	f.Add([]byte("a\tb\tc\none\ttwo\tthree\n"))
	f.Add([]byte("a:b:c\n"))
	f.Add([]byte("no-delim\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	flagSets := [][]string{
		{"-f", "1"},            // tab-delimited (default)
		{"-f", "1,3"},          // multiple fields
		{"-f", "2-"},           // range from field 2 onward
		{"-d", ":", "-f", "2"}, // colon delimiter
		{"-s", "-f", "1"},      // suppress lines without delimiter
		{"-c", "1-3"},          // character mode
		{"-b", "1-3"},          // byte mode
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		for _, args := range flagSets {
			testutil.FuzzCompare(t, "cut", cut.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
		}
	})
}
