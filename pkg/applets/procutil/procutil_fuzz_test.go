package procutil

import "testing"

// FuzzParseSignal fuzzes the signal parser with arbitrary signal names
// and numbers to ensure it never panics.
func FuzzParseSignal(f *testing.F) {
	seeds := []string{
		"-9", "9", "HUP", "SIGINT", "TERM", "-SIGKILL", "",
		"0", "15", "SIGHUP", "sigterm", "99", "-0",
		"USR1", "USR2", "PIPE", "ALRM",
		"not-a-signal", "12345", "-",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = ParseSignal(input)
	})
}

// FuzzLookupUser fuzzes the UID-to-username lookup to ensure it
// handles arbitrary UID strings without panicking.
func FuzzLookupUser(f *testing.F) {
	f.Add("0")
	f.Add("1000")
	f.Add("65534")
	f.Add("")
	f.Add("not-a-number")
	f.Add("-1")
	f.Add("4294967295")
	f.Fuzz(func(t *testing.T, uid string) {
		_ = LookupUser(uid)
	})
}
