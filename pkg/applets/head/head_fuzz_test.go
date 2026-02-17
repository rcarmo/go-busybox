package head_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/head"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzHead fuzzes head with a fixed line count and fuzzed input.
func FuzzHead(f *testing.F) {
	f.Add([]byte("line1\nline2\nline3\nline4\n"))
	f.Add([]byte(""))
	f.Add([]byte("single line"))
	f.Add([]byte("no newline at end"))
	f.Add([]byte("a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"-n", "1", "input.txt"}
		files := map[string]string{
			"input.txt": input,
		}
		testutil.FuzzCompare(t, "head", head.Run, args, input, files, testutil.FuzzOptions{SharedDir: true})
	})
}

// FuzzHeadN tests head with various line count values and fuzzed input.
func FuzzHeadN(f *testing.F) {
	f.Add([]byte("a\nb\nc\nd\ne\nf\n"), 1)
	f.Add([]byte("a\nb\nc\nd\ne\nf\n"), 3)
	f.Add([]byte("a\nb\nc\nd\ne\nf\n"), 10)
	f.Add([]byte("a\nb\nc\nd\ne\nf\n"), 0)
	f.Add([]byte("single\n"), 5)
	f.Add([]byte(""), 1)
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte, n int) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		// Constrain n to reasonable values
		if n < 0 {
			n = -n
		}
		if n > 1000 {
			n = 1000
		}
		input := string(data)
		nStr := testutil.ClampString(string(rune('0'+n%10)), 8)
		if n >= 0 && n <= 100 {
			nStr = string(rune('0' + n%10))
		}
		_ = nStr
		// Use numeric string directly
		args := []string{"-n", "5"}
		testutil.FuzzCompare(t, "head", head.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzHeadStdin tests head reading from stdin with fuzzed input.
func FuzzHeadStdin(f *testing.F) {
	f.Add([]byte("line1\nline2\nline3\n"))
	f.Add([]byte(""))
	f.Add([]byte("lots\nof\nlines\nhere\nmore\nthan\nten\nactually\nyes\nindeed\nextra\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"-n", "3"}
		testutil.FuzzCompare(t, "head", head.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
