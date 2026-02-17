package printf_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/printf"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzPrintf fuzzes the printf format string with fixed arguments to test
// the format parser robustness.
func FuzzPrintf(f *testing.F) {
	// Valid format strings that consume arguments
	f.Add("hello %s\n")
	f.Add("%d\n")
	f.Add("%05d\n")
	f.Add("%s %s %s\n")
	f.Add("%x\n")
	f.Add("%o\n")
	f.Add("%c")
	// Format strings without argument consumption (safe with no extra args)
	f.Add("no format\n")
	f.Add("")
	f.Add("\\n")
	f.Add("\\t")
	f.Add("\\\\")
	f.Add("\\x41")
	f.Add("\\0101")
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, format string) {
		format = testutil.ClampString(format, 256)
		// Only pass extra args if the format contains a format specifier
		// to avoid infinite loops when no args are consumed
		var args []string
		if containsFormatSpec(format) {
			args = []string{format, "hello", "42", "world"}
		} else {
			args = []string{format}
		}
		testutil.FuzzCompare(t, "printf", printf.Run, args, "", nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// containsFormatSpec returns true if the format string contains at least
// one format specifier (e.g., %s, %d) that would consume an argument.
func containsFormatSpec(format string) bool {
	for i := 0; i < len(format)-1; i++ {
		if format[i] == '%' {
			next := format[i+1]
			if next != '%' && next != 0 {
				return true
			}
			i++ // skip %%
		}
	}
	return false
}

// FuzzPrintfArgs fuzzes the arguments to printf with a fixed format string.
func FuzzPrintfArgs(f *testing.F) {
	f.Add([]byte("hello"))
	f.Add([]byte(""))
	f.Add([]byte("42"))
	f.Add([]byte("3.14"))
	f.Add([]byte("-1"))
	f.Add([]byte("0x1f"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, 128)
		arg := string(data)
		args := []string{"%s\\n", arg}
		testutil.FuzzCompare(t, "printf", printf.Run, args, "", nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
