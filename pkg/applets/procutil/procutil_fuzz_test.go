package procutil

import "testing"

func FuzzParseSignal(f *testing.F) {
	seeds := []string{"-9", "9", "HUP", "SIGINT", "TERM", "-SIGKILL", ""}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = ParseSignal(input)
	})
}
