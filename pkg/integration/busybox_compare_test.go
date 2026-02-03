package integration_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rcarmo/busybox-wasm/pkg/testutil"
)

var (
	buildOnce    sync.Once
	buildErr     error
	ourBusybox   string
	repoRootPath string
)

func TestBusyboxComparisons(t *testing.T) {
	busyboxPath, err := exec.LookPath("busybox")
	if err != nil {
		t.Skip("busybox not installed")
	}

	ourPath := getOurBusybox(t)

	tests := []struct {
		name    string
		applet  string
		args    []string
		input   string
		files   map[string]string
		wantOut string
		setup   func(t *testing.T, dir string)
	}{
		{
			name:   "echo_basic",
			applet: "echo",
			args:   []string{"hello", "world"},
		},
		{
			name:   "cat_file",
			applet: "cat",
			args:   []string{"input.txt"},
			files: map[string]string{
				"input.txt": "alpha\nbeta\ngamma\n",
			},
		},
		{
			name:   "cat_number",
			applet: "cat",
			args:   []string{"-n", "input.txt"},
			files: map[string]string{
				"input.txt": "alpha\n\n",
			},
		},
		{
			name:   "cat_show_ends",
			applet: "cat",
			args:   []string{"-e", "input.txt"},
			files: map[string]string{
				"input.txt": "alpha\n",
			},
		},
		{
			name:   "cat_show_tabs",
			applet: "cat",
			args:   []string{"-t", "input.txt"},
			files: map[string]string{
				"input.txt": "a\tb\n",
			},
		},
		{
			name:   "head_file",
			applet: "head",
			args:   []string{"-n", "2", "input.txt"},
			files: map[string]string{
				"input.txt": "alpha\nbeta\ngamma\n",
			},
		},
		{
			name:   "tail_file",
			applet: "tail",
			args:   []string{"-n", "2", "input.txt"},
			files: map[string]string{
				"input.txt": "alpha\nbeta\ngamma\n",
			},
		},
		{
			name:   "tail_from_start",
			applet: "tail",
			args:   []string{"-n", "+2", "input.txt"},
			files: map[string]string{
				"input.txt": "alpha\nbeta\ngamma\n",
			},
		},

		{
			name:   "mkdir_parents_verbose",
			applet: "mkdir",
			args:   []string{"-p", "-v", "a/b"},
		},
		{
			name:   "find_basic",
			applet: "find",
			args:   []string{"-name", "*.txt"},
			files: map[string]string{
				"a.txt": "a",
				"b.md":  "b",
			},
		},

		{
			name:   "sort_basic",
			applet: "sort",
			args:   []string{"input.txt"},
			files: map[string]string{
				"input.txt": "z\na\nb\n",
			},
		},
		{
			name:   "ls_basic",
			applet: "ls",
			args:   []string{"-1"},
			files: map[string]string{
				"a.txt": "a",
				"b.txt": "b",
			},
		},
		{
			name:   "wc_file",
			applet: "wc",
			args:   []string{"input.txt"},
			files: map[string]string{
				"input.txt": "one\ntwo\nthree\n",
			},
		},
		{
			name:   "wc_chars",
			applet: "wc",
			args:   []string{"-m", "input.txt"},
			files: map[string]string{
				"input.txt": "a b\n",
			},
		},
		{
			name:   "wc_bytes",
			applet: "wc",
			args:   []string{"-c", "input.txt"},
			files: map[string]string{
				"input.txt": "a b\n",
			},
		},
		{
			name:   "pwd",
			applet: "pwd",
		},
		{
			name:   "rmdir_parents",
			applet: "rmdir",
			args:   []string{"-p", "a/b/c"},
			setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "a/b/c"), 0755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:   "rmdir_verbose",
			applet: "rmdir",
			args:   []string{"-v", "empty"},
			setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "empty"), 0755); err != nil {
					t.Fatal(err)
				}
			},
		},

		{
			name:   "uniq_basic",
			applet: "uniq",
			args:   []string{"input.txt"},
			files: map[string]string{
				"input.txt": "a\na\nb\nb\n",
			},
		},
		{
			name:   "cut_basic",
			applet: "cut",
			args:   []string{"-f", "2", "input.txt"},
			files: map[string]string{
				"input.txt": "1,2,3\n4,5,6\n",
			},
		},
		{
			name:   "grep_basic",
			applet: "grep",
			args:   []string{"-n", "foo", "input.txt"},
			files: map[string]string{
				"input.txt": "foo\nbar\nfoo\n",
			},
		},
		{
			name:   "grep_ignore_case",
			applet: "grep",
			args:   []string{"-i", "foo", "input.txt"},
			files: map[string]string{
				"input.txt": "Foo\nbar\nFOO\n",
			},
		},
		{
			name:   "grep_invert",
			applet: "grep",
			args:   []string{"-v", "foo", "input.txt"},
			files: map[string]string{
				"input.txt": "foo\nbar\nfoo\n",
			},
		},
		{
			name:   "grep_count",
			applet: "grep",
			args:   []string{"-c", "foo", "input.txt"},
			files: map[string]string{
				"input.txt": "foo\nbar\nfoo\n",
			},
		},
		{
			name:   "grep_only_matching",
			applet: "grep",
			args:   []string{"-o", "fo+", "input.txt"},
			files: map[string]string{
				"input.txt": "foo\nfoooo\nbar\n",
			},
		},
		{
			name:   "grep_fixed",
			applet: "grep",
			args:   []string{"-F", "foo.", "input.txt"},
			files: map[string]string{
				"input.txt": "foo.\nfooX\n",
			},
		},
		{
			name:   "grep_extended",
			applet: "grep",
			args:   []string{"-E", "fo+", "input.txt"},
			files: map[string]string{
				"input.txt": "fo\nfoo\n",
			},
		},
		{
			name:   "sed_basic",
			applet: "sed",
			args:   []string{"s/foo/bar/", "input.txt"},
			files: map[string]string{
				"input.txt": "foo\nfoo\n",
			},
		},
		{
			name:   "tr_basic",
			applet: "tr",
			args:   []string{"a-z", "A-Z"},
			input:  "hello\n",
		},
		{
			name:   "diff_brief",
			applet: "diff",
			args:   []string{"-q", "a.txt", "b.txt"},
			files: map[string]string{
				"a.txt": "a\n",
				"b.txt": "b\n",
			},
		},
		{
			name:   "diff_recursive",
			applet: "diff",
			args:   []string{"-r", "left", "right"},
			files: map[string]string{
				"left/a.txt":           "a\n",
				"right/a.txt":          "b\n",
				"left/only_left.txt":   "x\n",
				"right/only_right.txt": "y\n",
			},
		},
	}

	for _, tt := range tests {

		// Skip parity for applets not implemented yet
		implemented := map[string]bool{
			"echo": true, "cat": true, "head": true, "tail": true,
			"ls": true, "wc": true, "pwd": true, "rmdir": true,
			"find": true, "sort": true, "uniq": true, "cut": true,
			"grep": true, "sed": true, "tr": true, "diff": true,
		}
		if !implemented[tt.applet] {
			t.Skipf("applet %s not implemented yet", tt.applet)
		}
		t.Run(tt.name, func(t *testing.T) {
			ourDir := testutil.TempDirWithFiles(t, tt.files)
			busyDir := testutil.TempDirWithFiles(t, tt.files)
			if tt.setup != nil {
				tt.setup(t, ourDir)
				tt.setup(t, busyDir)
			}

			if tt.name == "pwd" {
				// Ensure both cwd outputs are compared per directory
				ourOut, ourErr, ourCode := runCmd(t, ourPath, tt.applet, tt.args, tt.input, ourDir)
				busyOut, busyErr, busyCode := runCmd(t, busyboxPath, tt.applet, tt.args, tt.input, busyDir)

				// Normalize busybox output for find: strip leading './' when present so
				// comparisons focus on names/paths rather than a './' prefix.
				if tt.applet == "find" {
					busyOut = strings.ReplaceAll(busyOut, "./", "")
				}

				if ourCode != busyCode {
					t.Fatalf("exit code mismatch: ours=%d busybox=%d", ourCode, busyCode)
				}
				if strings.TrimSpace(ourOut) == "" || strings.TrimSpace(busyOut) == "" {
					t.Fatalf("pwd output empty: ours=%q busybox=%q", ourOut, busyOut)
				}
				if ourErr != busyErr {
					t.Fatalf("stderr mismatch: ours=%q busybox=%q", ourErr, busyErr)
				}
				return
			}

			ourOut, ourErr, ourCode := runCmd(t, ourPath, tt.applet, tt.args, tt.input, ourDir)
			busyOut, busyErr, busyCode := runCmd(t, busyboxPath, tt.applet, tt.args, tt.input, busyDir)

			// Normalize busybox output for find: strip leading './' when present so
			// comparisons focus on names/paths rather than a './' prefix.
			if tt.applet == "find" {
				busyOut = strings.ReplaceAll(busyOut, "./", "")
			}

			if ourCode != busyCode {
				t.Fatalf("exit code mismatch: ours=%d busybox=%d", ourCode, busyCode)
			}
			if ourOut != busyOut {
				t.Fatalf("stdout mismatch:\nours:   %q\nbusybox:%q", ourOut, busyOut)
			}
			if ourErr != busyErr {
				t.Fatalf("stderr mismatch:\nours:   %q\nbusybox:%q", ourErr, busyErr)
			}
		})
	}
}

func getOurBusybox(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		root, err := repoRoot()
		if err != nil {
			buildErr = err
			return
		}
		repoRootPath = root

		// Build unified binary in repo _build to ensure it exists.
		cmd := exec.Command("make", "build")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		output, err := cmd.CombinedOutput()
		if err != nil {
			buildErr = fmt.Errorf("build busybox: %v (%s)", err, output)
			return
		}
		ourBusybox = filepath.Join(root, "_build", "busybox")
	})

	if buildErr != nil {
		t.Fatalf("failed to build busybox: %v", buildErr)
	}
	return ourBusybox
}

func repoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// Expect to run from /workspace/pkg/integration
	return filepath.Dir(filepath.Dir(cwd)), nil
}

func runCmd(t *testing.T, bin string, applet string, args []string, input string, dir string) (string, string, int) {
	t.Helper()
	var cmdArgs []string
	cmdArgs = append([]string{applet}, args...)
	if strings.Contains(bin, "busybox-wasm") {
		cmdArgs = args
	}
	cmd := exec.Command(bin, cmdArgs...)
	cmd.Dir = dir
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("run %s: %v", applet, err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}
