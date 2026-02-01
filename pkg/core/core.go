// Package core provides shared functionality for busybox applets.
package core

import (
	"fmt"
	"io"
	"os"
)

// Exit codes following POSIX conventions
const (
	ExitSuccess = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// Stdio holds the standard I/O streams for an applet.
// This allows for easy testing by injecting mock streams.
type Stdio struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// DefaultStdio returns Stdio configured with os.Stdin, os.Stdout, os.Stderr.
func DefaultStdio() *Stdio {
	return &Stdio{
		In:  os.Stdin,
		Out: os.Stdout,
		Err: os.Stderr,
	}
}

// Errorf writes a formatted error message to stderr.
func (s *Stdio) Errorf(format string, args ...any) {
	fmt.Fprintf(s.Err, format, args...)
}

// Printf writes a formatted message to stdout.
func (s *Stdio) Printf(format string, args ...any) {
	fmt.Fprintf(s.Out, format, args...)
}

// Print writes a message to stdout.
func (s *Stdio) Print(args ...any) {
	fmt.Fprint(s.Out, args...)
}

// Println writes a message to stdout with a newline.
func (s *Stdio) Println(args ...any) {
	fmt.Fprintln(s.Out, args...)
}

// UsageError prints a usage error and returns ExitUsage.
func UsageError(stdio *Stdio, applet, message string) int {
	stdio.Errorf("%s: %s\n", applet, message)
	return ExitUsage
}

// FileError prints a file-related error and returns ExitFailure.
func FileError(stdio *Stdio, applet, path string, err error) int {
	stdio.Errorf("%s: %s: %v\n", applet, path, err)
	return ExitFailure
}
