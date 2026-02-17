// command.go provides helpers for locating the busybox binary in tests.
package testutil

import "os/exec"

// Command wraps exec.Command for test helpers.
// Command returns an exec.Cmd for running a busybox applet, using the
// built binary at _build/busybox if available.
func Command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...) // #nosec G204 -- test helper for external command
}
