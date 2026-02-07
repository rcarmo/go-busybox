package testutil

import "os/exec"

// Command wraps exec.Command for test helpers.
func Command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...) // #nosec G204 -- test helper for external command
}
