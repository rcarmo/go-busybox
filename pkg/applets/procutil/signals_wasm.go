//go:build js || wasm || wasip1

package procutil

import "syscall"

// signalNames returns a reduced signal name table for WASM platforms.
// SIGHUP, SIGUSR1, SIGUSR2, and SIGALRM are unavailable under WASI.
func signalNames() map[syscall.Signal]string {
	return map[syscall.Signal]string{
		syscall.SIGINT:  "INT",
		syscall.SIGQUIT: "QUIT",
		syscall.SIGILL:  "ILL",
		syscall.SIGTRAP: "TRAP",
		syscall.SIGABRT: "ABRT",
		syscall.SIGBUS:  "BUS",
		syscall.SIGFPE:  "FPE",
		syscall.SIGKILL: "KILL",
		syscall.SIGSEGV: "SEGV",
		syscall.SIGPIPE: "PIPE",
		syscall.SIGTERM: "TERM",
	}
}
