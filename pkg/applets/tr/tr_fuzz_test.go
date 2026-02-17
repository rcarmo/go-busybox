package tr_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/tr"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzTr fuzzes tr with lowercase-to-uppercase translation and random input.
func FuzzTr(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte("ABC abc 123"))
	f.Add([]byte("aaabbbccc"))
	f.Add([]byte("\t\n "))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"a-z", "A-Z"}
		testutil.FuzzCompare(t, "tr", tr.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true})
	})
}

// FuzzTrDelete fuzzes tr in delete mode (-d) with character class operands.
func FuzzTrDelete(f *testing.F) {
	f.Add([]byte("hello world 123"))
	f.Add([]byte(""))
	f.Add([]byte("ALL CAPS"))
	f.Add([]byte("MiXeD CaSe 42"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"-d", "a-z"}
		testutil.FuzzCompare(t, "tr", tr.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzTrSqueeze fuzzes tr in squeeze mode (-s).
func FuzzTrSqueeze(f *testing.F) {
	f.Add([]byte("aaa   bbb   ccc"))
	f.Add([]byte(""))
	f.Add([]byte("   spaces   "))
	f.Add([]byte("no repeats"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"-s", " "}
		testutil.FuzzCompare(t, "tr", tr.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzTrComplement fuzzes tr with complement mode (-c).
func FuzzTrComplement(f *testing.F) {
	f.Add([]byte("hello world 123"))
	f.Add([]byte(""))
	f.Add([]byte("ABCabc123!@#"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"-cd", "a-zA-Z"}
		testutil.FuzzCompare(t, "tr", tr.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzTrSet fuzzes the SET1 and SET2 operands themselves.
func FuzzTrSet(f *testing.F) {
	f.Add("a-z", "A-Z")
	f.Add("aeiou", "AEIOU")
	f.Add("0-9", "a-j")
	f.Add("abc", "xyz")
	f.Add("a", "b")
	f.Add("[:lower:]", "[:upper:]")
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, set1, set2 string) {
		set1 = testutil.ClampString(set1, 64)
		set2 = testutil.ClampString(set2, 64)
		if set1 == "" {
			return
		}
		input := "Hello World 123 foo BAR\n"
		args := []string{set1, set2}
		testutil.FuzzCompare(t, "tr", tr.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
