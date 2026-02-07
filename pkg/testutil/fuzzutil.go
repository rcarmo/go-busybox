package testutil

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

const MaxFuzzBytes = 2048

type FuzzOptions struct {
	SkipBusybox bool
	SharedDir   bool
}

var cwdMu sync.Mutex

func ClampBytes(data []byte, max int) []byte {
	if len(data) > max {
		return data[:max]
	}
	return data
}

func ClampString(data string, max int) string {
	if len(data) > max {
		return data[:max]
	}
	return data
}

func RunAppletInDir(t *testing.T, run RunApplet, args []string, input string, dir string) (string, string, int) {
	t.Helper()
	cwdMu.Lock()
	defer cwdMu.Unlock()

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldDir) }()

	stdio, out, errBuf := CaptureStdio(input)
	code := run(stdio, args)
	return out.String(), errBuf.String(), code
}

func RunBusyboxInDir(t *testing.T, applet string, args []string, input string, dir string) (string, string, int, bool) {
	t.Helper()
	busyboxPath, err := exec.LookPath("busybox")
	if err != nil {
		return "", "", 0, false
	}
	cmdArgs := append([]string{applet}, args...)
	cmd := exec.Command(busyboxPath, cmdArgs...)
	cmd.Dir = dir
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("busybox run %s: %v", applet, err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode, true
}

func FuzzCompare(t *testing.T, applet string, run RunApplet, args []string, input string, files map[string]string, opts FuzzOptions) {
	t.Helper()
	if opts.SharedDir {
		dir := TempDirWithFiles(t, files)
		ourOut, ourErr, ourCode := RunAppletInDir(t, run, args, input, dir)
		if opts.SkipBusybox {
			_ = ourOut
			_ = ourErr
			_ = ourCode
			return
		}
		busyOut, busyErr, busyCode, ok := RunBusyboxInDir(t, applet, args, input, dir)
		if !ok {
			return
		}
		CompareBusyboxOutput(t, applet, ourOut, ourErr, ourCode, busyOut, busyErr, busyCode)
		return
	}
	ourDir := TempDirWithFiles(t, files)
	busyDir := TempDirWithFiles(t, files)
	ourOut, ourErr, ourCode := RunAppletInDir(t, run, args, input, ourDir)
	if opts.SkipBusybox {
		_ = ourOut
		_ = ourErr
		_ = ourCode
		return
	}
	busyOut, busyErr, busyCode, ok := RunBusyboxInDir(t, applet, args, input, busyDir)
	if !ok {
		return
	}
	CompareBusyboxOutput(t, applet, ourOut, ourErr, ourCode, busyOut, busyErr, busyCode)
}

func NormalizePsOutput(out string) string {
	lines := strings.Split(out, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.Contains(line, "busybox") {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

func CompareBusyboxOutput(t *testing.T, applet string, ourOut, ourErr string, ourCode int, busyOut, busyErr string, busyCode int) {
	t.Helper()
	if applet == "ps" {
		ourOut = NormalizePsOutput(ourOut)
		busyOut = NormalizePsOutput(busyOut)
	}

	if applet == "find" {
		busyOut = strings.ReplaceAll(busyOut, "./", "")
	}
	if ourCode != busyCode {
		if busyCode == 1 && ourCode == 2 && (isUsageError(ourErr) || isUsageError(busyErr)) {
			return
		}
		t.Fatalf("exit code mismatch: ours=%d busybox=%d", ourCode, busyCode)
	}
	if !outputsEqual(ourOut, busyOut) {
		t.Fatalf("stdout mismatch:\nours:   %q\nbusybox:%q", ourOut, busyOut)
	}
	if !outputsEqual(ourErr, busyErr) {
		if isUsageError(ourErr) || isUsageError(busyErr) {
			return
		}
		t.Fatalf("stderr mismatch:\nours:   %q\nbusybox:%q", ourErr, busyErr)
	}
}

func isUsageError(err string) bool {
	if err == "" {
		return false
	}
	return strings.Contains(err, "invalid option") ||
		strings.Contains(err, "missing") ||
		strings.Contains(err, "unrecognized")
}

func outputsEqual(a, b string) bool {
	if a == b {
		return true
	}
	trimA := strings.TrimSuffix(a, "\n")
	trimB := strings.TrimSuffix(b, "\n")
	return trimA == trimB
}
