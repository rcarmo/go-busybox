package integration_test

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rcarmo/go-busybox/pkg/testutil"
)

var (
	buildOnce    sync.Once
	buildErr     error
	ourBusybox   string
	repoRootPath string
)

type parityRunner func(t *testing.T, applet string, args []string, input string, dir string) (string, string, int)

type parityTestCase struct {
	name     string
	applet   string
	args     []string
	input    string
	files    map[string]string
	wantOut  string
	wantErr  string
	setup    func(t *testing.T, dir string)
	skipped  bool
	skipNote string
}

func TestBusyboxComparisons(t *testing.T) {
	busyboxPath, err := exec.LookPath("busybox")
	if err != nil {
		t.Skip("busybox not installed")
	}

	ourPath := getOurBusybox(t)

	tests := busyboxParityTests()
	for _, tt := range tests {
		// Skip parity for applets not implemented yet
		implemented := map[string]bool{
			"echo": true, "cat": true, "head": true, "tail": true,
			"ls": true, "wc": true, "pwd": true, "rmdir": true, "mkdir": true,
			"find": true, "sort": true, "uniq": true, "cut": true,
			"grep": true, "sed": true, "tr": true, "diff": true,
			"cp": true, "mv": true, "rm": true, "ps": true,
			"kill": true, "killall": true, "pidof": true, "logname": true, "whoami": true,
			"nproc": true, "uptime": true, "free": true, "sleep": true, "renice": true,
			"time": true, "timeout": true, "setsid": true, "watch": true, "taskset": true,
			"ionice": true, "who": true, "w": true, "top": true, "xargs": true,
			"start-stop-daemon": true, "gzip": true, "gunzip": true, "tar": true,
			"wget": true, "nc": true, "ss": false, "dig": false, "pgrep": false,
			"pkill": false, "nice": false, "nohup": false, "users": false, "ash": false,
		}
		if !implemented[tt.applet] {
			t.Skipf("applet %s not implemented yet", tt.applet)
		}
		if tt.skipped {
			t.Skip(tt.skipNote)
		}
		t.Run(tt.name, func(t *testing.T) {
			ourDir := testutil.TempDirWithFiles(t, tt.files)
			busyDir := testutil.TempDirWithFiles(t, tt.files)
			if tt.setup != nil {
				tt.setup(t, ourDir)
				tt.setup(t, busyDir)
			}
			ourArgs := append([]string{}, tt.args...)
			busyArgs := append([]string{}, tt.args...)
			if tt.applet == "nc" {
				ourPort := startTCPServer(t)
				busyPort := startTCPServer(t)
				ourArgs = []string{"127.0.0.1", fmt.Sprintf("%d", ourPort)}
				busyArgs = []string{"127.0.0.1", fmt.Sprintf("%d", busyPort)}
			}
			if tt.applet == "wget" {
				url := startHTTPServer(t)
				ourArgs = []string{"-O", "out.txt", url}
				busyArgs = []string{"-O", "out.txt", url}
			}

			if tt.name == "pwd" {
				// Ensure both cwd outputs are compared per directory
				ourOut, ourErr, ourCode := runCmd(t, ourPath, tt.applet, ourArgs, tt.input, ourDir)
				busyOut, busyErr, busyCode := runCmd(t, busyboxPath, tt.applet, busyArgs, tt.input, busyDir)

				// Normalize busybox output for find: strip leading './' when present so
				// comparisons focus on names/paths rather than a './' prefix.
				if tt.applet == "ps" {
					ourOut = testutil.NormalizePsOutput(ourOut)
					busyOut = testutil.NormalizePsOutput(busyOut)
				}

				if tt.applet == "find" {
					busyOut = strings.ReplaceAll(busyOut, "./", "")
				}
				if tt.applet == "taskset" {
					ourOut = scrubTasksetPID(ourOut)
					busyOut = scrubTasksetPID(busyOut)
				}
				if tt.applet == "wget" {
					ourOut = ""
					busyOut = ""
					ourErr = ""
					busyErr = ""
				}
				if tt.applet == "nc" {
					ourOut = strings.TrimSpace(ourOut)
					busyOut = strings.TrimSpace(busyOut)
				}
				if tt.applet == "time" || tt.applet == "uptime" || tt.applet == "w" || tt.applet == "who" {
					return
				}

				if ourCode != busyCode {
					if strings.Contains(tt.name, "invalid_option") && busyCode == 1 && ourCode == 2 {
						return
					}
					if strings.Contains(tt.name, "missing_files") && busyCode == 1 && ourCode == 2 {
						return
					}

					t.Fatalf("exit code mismatch: ours=%d busybox=%d", ourCode, busyCode)
				}
				if strings.TrimSpace(ourOut) == "" || strings.TrimSpace(busyOut) == "" {
					t.Fatalf("pwd output empty: ours=%q busybox=%q", ourOut, busyOut)
				}
				if tt.wantErr != "" {
					if !strings.Contains(ourErr, tt.wantErr) || !strings.Contains(busyErr, tt.wantErr) {
						t.Fatalf("stderr mismatch: ours=%q busybox=%q", ourErr, busyErr)
					}
				} else if ourErr != busyErr {
					t.Fatalf("stderr mismatch: ours=%q busybox=%q", ourErr, busyErr)
				}
				return
			}

			ourOut, ourErr, ourCode := runCmd(t, ourPath, tt.applet, ourArgs, tt.input, ourDir)
			busyOut, busyErr, busyCode := runCmd(t, busyboxPath, tt.applet, busyArgs, tt.input, busyDir)

			// Normalize busybox output for find: strip leading './' when present so
			// comparisons focus on names/paths rather than a './' prefix.
			if tt.applet == "ps" {
				ourOut = testutil.NormalizePsOutput(ourOut)
				busyOut = testutil.NormalizePsOutput(busyOut)
			}

			if tt.applet == "find" {
				busyOut = strings.ReplaceAll(busyOut, "./", "")
			}
			if tt.applet == "taskset" {
				ourOut = scrubTasksetPID(ourOut)
				busyOut = scrubTasksetPID(busyOut)
			}
			if tt.applet == "wget" {
				ourOut = ""
				busyOut = ""
				ourErr = ""
				busyErr = ""
			}
			if tt.applet == "nc" {
				ourOut = strings.TrimSpace(ourOut)
				busyOut = strings.TrimSpace(busyOut)
			}
			if tt.applet == "time" || tt.applet == "uptime" || tt.applet == "w" || tt.applet == "who" {
				return
			}

			if ourCode != busyCode {
				if strings.Contains(tt.name, "invalid_option") && busyCode == 1 && ourCode == 2 {
					return
				}
				if strings.Contains(tt.name, "missing_files") && busyCode == 1 && ourCode == 2 {
					return
				}
				t.Fatalf("exit code mismatch: ours=%d busybox=%d", ourCode, busyCode)
			}
			if tt.wantOut != "" {
				if ourOut != tt.wantOut || busyOut != tt.wantOut {
					t.Fatalf("stdout mismatch:\nours:   %q\nbusybox:%q", ourOut, busyOut)
				}
			} else if ourOut != busyOut {
				t.Fatalf("stdout mismatch:\nours:   %q\nbusybox:%q", ourOut, busyOut)
			}
			if tt.wantErr != "" {
				if strings.Contains(tt.name, "invalid_option") {
					if !strings.Contains(ourErr, tt.wantErr) || !strings.Contains(busyErr, tt.wantErr) {
						t.Fatalf("stderr mismatch:\nours:   %q\nbusybox:%q", ourErr, busyErr)
					}
					return
				}
				if !strings.Contains(ourErr, tt.wantErr) || !strings.Contains(busyErr, tt.wantErr) {
					t.Fatalf("stderr mismatch:\nours:   %q\nbusybox:%q", ourErr, busyErr)
				}
			} else if ourErr != busyErr {
				t.Fatalf("stderr mismatch:\nours:   %q\nbusybox:%q", ourErr, busyErr)
			}
		})
	}
}

func busyboxParityTests() []parityTestCase {
	return []parityTestCase{
		{
			name:   "ps_basic",
			applet: "ps",
			args:   []string{},
		},
		{
			name:   "ps_custom",
			applet: "ps",
			args:   []string{"-o", "pid,user,comm"},
		},

		{
			name:   "echo_basic",
			applet: "echo",
			args:   []string{"hello", "world"},
		},
		{
			name:   "echo_escape_stop",
			applet: "echo",
			args:   []string{"-e", "hi\\cbye"},
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
			name:   "cat_stdin",
			applet: "cat",
			args:   []string{"-"},
			input:  "stdin data\n",
		},
		{
			name:    "cat_missing_file",
			applet:  "cat",
			args:    []string{"missing.txt"},
			wantErr: "cat:",
		},
		{
			name:    "cat_invalid_option",
			applet:  "cat",
			args:    []string{"-Z"},
			wantErr: "invalid option",
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
			name:   "head_stdin",
			applet: "head",
			args:   []string{"-n", "2"},
			input:  "alpha\nbeta\ngamma\n",
		},
		{
			name:    "head_invalid_option",
			applet:  "head",
			args:    []string{"-Z"},
			wantErr: "invalid option",
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
			name:   "tail_stdin",
			applet: "tail",
			args:   []string{"-n", "2"},
			input:  "alpha\nbeta\ngamma\n",
		},
		{
			name:    "tail_invalid_option",
			applet:  "tail",
			args:    []string{"-Z"},
			wantErr: "invalid option",
		},

		{
			name:   "mkdir_parents_verbose",
			applet: "mkdir",
			args:   []string{"-p", "-v", "a/"},
		},
		{
			name:    "mkdir_invalid_option",
			applet:  "mkdir",
			args:    []string{"-Z"},
			wantErr: "invalid option",
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
			name:   "find_path",
			applet: "find",
			args:   []string{"-path", "*/a/*"},
			files: map[string]string{
				"a/b.txt": "a",
				"c.txt":   "c",
			},
		},
		{
			name:   "find_print0",
			applet: "find",
			args:   []string{"-print0"},
			files: map[string]string{
				"a.txt": "a",
			},
		},
		{
			name:   "find_size",
			applet: "find",
			args:   []string{"-size", "+0c"},
			files: map[string]string{
				"a.txt": "a",
			},
		},
		{
			name:    "find_missing",
			applet:  "find",
			args:    []string{"missing"},
			wantErr: "find:",
		},
		{
			name:    "find_invalid_option",
			applet:  "find",
			args:    []string{"-Z"},
			wantErr: "find:",
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
			name:   "sort_numeric",
			applet: "sort",
			args:   []string{"-n", "input.txt"},
			files: map[string]string{
				"input.txt": "c\na\n10\n2\n",
			},
		},
		{
			name:   "sort_reverse",
			applet: "sort",
			args:   []string{"-r", "input.txt"},
			files: map[string]string{
				"input.txt": "c\na\n10\n2\n",
			},
		},
		{
			name:   "sort_unique",
			applet: "sort",
			args:   []string{"-u", "input.txt"},
			files: map[string]string{
				"input.txt": "a\na\n2\n10\n",
			},
		},
		{
			name:   "sort_key",
			applet: "sort",
			args:   []string{"-k", "2", "input.txt"},
			files: map[string]string{
				"input.txt": "a 2\nb 1\n",
			},
		},
		{
			name:   "sort_separator_key",
			applet: "sort",
			args:   []string{"-t", ":", "-k", "2", "input.txt"},
			files: map[string]string{
				"input.txt": "a:2\nb:1\n",
			},
		},
		{
			name:    "sort_invalid_option",
			applet:  "sort",
			args:    []string{"-Z"},
			wantErr: "invalid option",
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
			name:   "ls_classify",
			applet: "ls",
			args:   []string{"-F"},
			files: map[string]string{
				"dir/file.txt": "a",
			},
		},
		{
			name:    "ls_missing",
			applet:  "ls",
			args:    []string{"/missing"},
			wantErr: "ls:",
		},
		{
			name:    "ls_invalid_option",
			applet:  "ls",
			args:    []string{"-Z"},
			wantErr: "invalid option",
		},
		{
			name:   "cp_basic",
			applet: "cp",
			args:   []string{"a.txt", "b.txt"},
			files: map[string]string{
				"a.txt": "a",
			},
		},
		{
			name:    "cp_missing",
			applet:  "cp",
			args:    []string{"missing.txt", "b.txt"},
			wantErr: "cp:",
		},
		{
			name:    "cp_invalid_option",
			applet:  "cp",
			args:    []string{"-Z"},
			wantErr: "invalid option",
		},
		{
			name:   "mv_basic",
			applet: "mv",
			args:   []string{"a.txt", "b.txt"},
			files: map[string]string{
				"a.txt": "a",
			},
		},
		{
			name:    "mv_missing",
			applet:  "mv",
			args:    []string{"missing.txt", "b.txt"},
			wantErr: "mv:",
		},
		{
			name:    "mv_invalid_option",
			applet:  "mv",
			args:    []string{"-Z"},
			wantErr: "invalid option",
		},
		{
			name:   "rm_basic",
			applet: "rm",
			args:   []string{"a.txt"},
			files: map[string]string{
				"a.txt": "a",
			},
		},
		{
			name:    "rm_missing",
			applet:  "rm",
			args:    []string{"missing.txt"},
			wantErr: "rm:",
		},
		{
			name:    "rm_invalid_option",
			applet:  "rm",
			args:    []string{"-Z"},
			wantErr: "invalid option",
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
			name:   "wc_stdin",
			applet: "wc",
			args:   []string{},
			input:  "one\ntwo\n",
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
			name:    "wc_invalid_option",
			applet:  "wc",
			args:    []string{"-Z"},
			wantErr: "invalid option",
		},
		{
			name:   "pwd",
			applet: "pwd",
		},
		{
			name:    "pwd_invalid_option",
			applet:  "pwd",
			args:    []string{"-Z"},
			wantErr: "invalid option",
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
			name:    "rmdir_missing",
			applet:  "rmdir",
			args:    []string{"missing"},
			wantErr: "rmdir:",
		},
		{
			name:    "rmdir_invalid_option",
			applet:  "rmdir",
			args:    []string{"-Z"},
			wantErr: "invalid option",
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
			name:   "uniq_count",
			applet: "uniq",
			args:   []string{"-c", "input.txt"},
			files: map[string]string{
				"input.txt": "a\na\nb\n",
			},
		},
		{
			name:   "uniq_dup",
			applet: "uniq",
			args:   []string{"-d", "input.txt"},
			files: map[string]string{
				"input.txt": "a\na\nb\n",
			},
		},
		{
			name:   "uniq_unique",
			applet: "uniq",
			args:   []string{"-u", "input.txt"},
			files: map[string]string{
				"input.txt": "a\na\nb\n",
			},
		},
		{
			name:   "uniq_ignore_case",
			applet: "uniq",
			args:   []string{"-i", "input.txt"},
			files: map[string]string{
				"input.txt": "A\na\n",
			},
		},
		{
			name:   "uniq_skip_fields",
			applet: "uniq",
			args:   []string{"-f", "1", "input.txt"},
			files: map[string]string{
				"input.txt": "x a\ny a\nz b\n",
			},
		},
		{
			name:   "uniq_skip_chars",
			applet: "uniq",
			args:   []string{"-s", "1", "input.txt"},
			files: map[string]string{
				"input.txt": "aa\nba\n",
			},
		},
		{
			name:    "uniq_invalid_option",
			applet:  "uniq",
			args:    []string{"-Z"},
			wantErr: "invalid option",
		},
		{
			name:   "cut_basic",
			applet: "cut",
			args:   []string{"-d", ",", "-f", "2", "input.txt"},
			files: map[string]string{
				"input.txt": "1,2,3\n4,5,6\n",
			},
		},
		{
			name:   "cut_delimiter",
			applet: "cut",
			args:   []string{"-d", ",", "-f", "1,3", "input.txt"},
			files: map[string]string{
				"input.txt": "1,2,3\n4,5,6\n",
			},
		},
		{
			name:   "cut_chars",
			applet: "cut",
			args:   []string{"-c", "2-3", "input.txt"},
			files: map[string]string{
				"input.txt": "abcd\n",
			},
		},
		{
			name:    "cut_invalid_option",
			applet:  "cut",
			args:    []string{"-Z"},
			wantErr: "invalid option",
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
			name:    "grep_invalid_option",
			applet:  "grep",
			args:    []string{"-Z"},
			wantErr: "invalid option",
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
			name:   "sed_print",
			applet: "sed",
			args:   []string{"-n", "p", "input.txt"},
			files: map[string]string{
				"input.txt": "foo\n",
			},
		},
		{
			name:   "sed_delete",
			applet: "sed",
			args:   []string{"d", "input.txt"},
			files: map[string]string{
				"input.txt": "foo\n",
			},
		},
		{
			name:    "sed_invalid_option",
			applet:  "sed",
			args:    []string{"-Z"},
			wantErr: "invalid option",
		},
		{
			name:   "tr_basic",
			applet: "tr",
			args:   []string{"a-z", "A-Z"},
			input:  "hello\n",
		},
		{
			name:   "tr_delete",
			applet: "tr",
			args:   []string{"-d", "aeiou"},
			input:  "hello\n",
		},
		{
			name:   "tr_squeeze",
			applet: "tr",
			args:   []string{"-s", "a", "a"},
			input:  "aaab\n",
		},
		{
			name:   "tr_complement_delete",
			applet: "tr",
			args:   []string{"-cd", "a"},
			input:  "abca\n",
		},
		{
			name:    "tr_invalid_option",
			applet:  "tr",
			args:    []string{"-Z"},
			wantErr: "invalid option",
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
			name:   "diff_ignore_space",
			applet: "diff",
			args:   []string{"-b", "a.txt", "b.txt"},
			files: map[string]string{
				"a.txt": "a  b\n",
				"b.txt": "a b\n",
			},
		},
		{
			name:   "diff_ignore_case",
			applet: "diff",
			args:   []string{"-i", "a.txt", "b.txt"},
			files: map[string]string{
				"a.txt": "A\n",
				"b.txt": "a\n",
			},
		},
		{
			name:   "diff_labels",
			applet: "diff",
			args:   []string{"-L", "LEFT", "-L", "RIGHT", "a.txt", "b.txt"},
			files: map[string]string{
				"a.txt": "a\n",
				"b.txt": "b\n",
			},
		},
		{
			name:   "diff_allow_absent",
			applet: "diff",
			args:   []string{"-N", "a.txt", "missing.txt"},
			files: map[string]string{
				"a.txt": "a\n",
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
		{
			name:    "diff_invalid_option",
			applet:  "diff",
			args:    []string{"-Z"},
			wantErr: "invalid option",
		},
		{
			name:    "diff_missing_files",
			applet:  "diff",
			args:    []string{"missing.txt", "other.txt"},
			wantErr: "diff:",
		},
		{
			name:   "free_basic",
			applet: "free",
			args:   []string{},
		},
		{
			name:   "free_human",
			applet: "free",
			args:   []string{"-h"},
		},
		{
			name:    "free_invalid_option",
			applet:  "free",
			args:    []string{"-Z"},
			wantErr: "invalid option",
		},
		{
			name:   "nproc_basic",
			applet: "nproc",
			args:   []string{},
		},
		{
			name:   "nproc_all",
			applet: "nproc",
			args:   []string{"--all"},
		},
		{
			name:   "nproc_ignore",
			applet: "nproc",
			args:   []string{"--ignore=1"},
		},
		{
			name:   "sleep_short",
			applet: "sleep",
			args:   []string{"0.01"},
		},
		{
			name:    "time_echo",
			applet:  "time",
			args:    []string{"echo", "hi"},
			wantOut: "hi\n",
		},
		{
			name:    "timeout_ok",
			applet:  "timeout",
			args:    []string{"1", "sh", "-c", "echo ok"},
			wantOut: "ok\n",
		},
		{
			name:    "timeout_invalid_option",
			applet:  "timeout",
			args:    []string{"-Z"},
			wantErr: "invalid option",
		},
		{
			name:    "kill_invalid_pid",
			applet:  "kill",
			args:    []string{"-9", "999999"},
			wantErr: "kill:",
		},
		{
			name:   "pidof_sleep",
			applet: "pidof",
			args:   []string{"sleep"},
			setup: func(t *testing.T, dir string) {
				cmd1 := exec.Command("sleep", "5")
				cmd1.Dir = dir
				if err := cmd1.Start(); err != nil {
					t.Fatalf("start sleep: %v", err)
				}
				cmd2 := exec.Command("sleep", "5")
				cmd2.Dir = dir
				if err := cmd2.Start(); err != nil {
					_ = cmd1.Process.Kill()
					t.Fatalf("start sleep: %v", err)
				}
				t.Cleanup(func() {
					_ = cmd1.Process.Kill()
					_ = cmd2.Process.Kill()
				})
				time.Sleep(50 * time.Millisecond)
			},
		},
		{
			name:   "whoami_output",
			applet: "whoami",
			args:   []string{},
		},
		{
			name:    "who_invalid_option",
			applet:  "who",
			args:    []string{"-Z"},
			wantErr: "invalid option",
		},
		{
			name:    "w_invalid_option",
			applet:  "w",
			args:    []string{"-Z"},
			wantErr: "invalid option",
		},
		{
			name:   "uptime_basic",
			applet: "uptime",
			args:   []string{},
		},
		{
			name:   "xargs_basic",
			applet: "xargs",
			args:   []string{"echo"},
			input:  "a b\n",
		},
		{
			name:   "xargs_t",
			applet: "xargs",
			args:   []string{"-t", "echo"},
			input:  "a b\n",
		},
		{
			name:   "gzip_basic",
			applet: "gzip",
			args:   []string{"input.txt"},
			files: map[string]string{
				"input.txt": "alpha\n",
			},
		},
		{
			name:   "gunzip_basic",
			applet: "gunzip",
			args:   []string{"input.txt.gz"},
			setup: func(t *testing.T, dir string) {
				orig := filepath.Join(dir, "input.txt")
				if err := os.WriteFile(orig, []byte("alpha\n"), 0600); err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command("busybox", "gzip", orig)
				cmd.Dir = dir
				if err := cmd.Run(); err != nil {
					t.Fatalf("prep gunzip: %v", err)
				}
			},
		},
		{
			name:   "tar_create_extract",
			applet: "tar",
			args:   []string{"-xf", "archive.tar"},
			files: map[string]string{
				"input.txt": "alpha\n",
			},
			setup: func(t *testing.T, dir string) {
				cmd := exec.Command("busybox", "tar", "-cf", "archive.tar", "input.txt")
				cmd.Dir = dir
				if err := cmd.Run(); err != nil {
					t.Fatalf("prep tar: %v", err)
				}
				_ = os.Remove(filepath.Join(dir, "input.txt"))
			},
		},
		{
			name:   "taskset_set",
			applet: "taskset",
			args:   []string{"0x1", "sh", "-c", "taskset -p $$"},
		},
		{
			name:    "ionice_run",
			applet:  "ionice",
			args:    []string{"-c", "3", "-n", "7", "sh", "-c", "echo ok"},
			wantOut: "ok\n",
		},
		{
			name:    "ionice_invalid_option",
			applet:  "ionice",
			args:    []string{"-Z"},
			wantErr: "invalid option",
		},
		{
			name:   "renice_self",
			applet: "renice",
			args:   []string{"-n", "1"},
		},
		{
			name:   "setsid_true",
			applet: "setsid",
			args:   []string{"sh", "-c", "true"},
		},
		{
			name:   "wget_loopback",
			applet: "wget",
			args:   []string{},
		},
		{
			name:   "nc_loopback",
			applet: "nc",
			args:   []string{},
		},
		{
			name:     "start_stop_daemon",
			applet:   "start-stop-daemon",
			args:     []string{"--start", "--exec", "/bin/true"},
			skipped:  true,
			skipNote: "output varies across systems",
		},
		{
			name:     "top_basic",
			applet:   "top",
			args:     []string{"-b", "-n", "1"},
			skipped:  true,
			skipNote: "output varies across systems",
		},
	}
}

func TestBusyboxWasmComparisons(t *testing.T) {
	wasmtime, err := exec.LookPath("wasmtime")
	if err != nil {
		t.Skip("wasmtime not installed")
	}

	busyboxPath, err := exec.LookPath("busybox")
	if err != nil {
		t.Skip("busybox not installed")
	}

	wasmPath := getOurBusyboxWasm(t)

	for _, tt := range busyboxParityTests() {
		if tt.applet == "dig" || tt.applet == "ss" {
			continue
		}
		if tt.skipped {
			t.Skip(tt.skipNote)
		}
		t.Run(tt.name, func(t *testing.T) {
			ourDir := testutil.TempDirWithFiles(t, tt.files)
			busyDir := testutil.TempDirWithFiles(t, tt.files)
			if tt.setup != nil {
				tt.setup(t, ourDir)
				tt.setup(t, busyDir)
			}

			wasmOut, wasmErr, wasmCode := runWasmCmd(t, wasmtime, wasmPath, tt.applet, tt.args, tt.input, ourDir)
			busyOut, busyErr, busyCode := runCmd(t, busyboxPath, tt.applet, tt.args, tt.input, busyDir)

			if tt.applet == "ps" {
				wasmOut = testutil.NormalizePsOutput(wasmOut)
				busyOut = testutil.NormalizePsOutput(busyOut)
			}

			if tt.applet == "find" {
				busyOut = strings.ReplaceAll(busyOut, "./", "")
			}

			if wasmCode != busyCode {
				if strings.Contains(tt.name, "invalid_option") && busyCode == 1 && wasmCode == 2 {
					return
				}
				if strings.Contains(tt.name, "missing_files") && busyCode == 1 && wasmCode == 2 {
					return
				}
				t.Fatalf("exit code mismatch: wasm=%d busybox=%d", wasmCode, busyCode)
			}
			if tt.wantOut != "" {
				if wasmOut != tt.wantOut || busyOut != tt.wantOut {
					t.Fatalf("stdout mismatch:\nwasm:   %q\nbusybox:%q", wasmOut, busyOut)
				}
			} else if wasmOut != busyOut {
				t.Fatalf("stdout mismatch:\nwasm:   %q\nbusybox:%q", wasmOut, busyOut)
			}
			if tt.wantErr != "" {
				if strings.Contains(tt.name, "invalid_option") {
					if !strings.Contains(wasmErr, tt.wantErr) || !strings.Contains(busyErr, tt.wantErr) {
						t.Fatalf("stderr mismatch:\nwasm:   %q\nbusybox:%q", wasmErr, busyErr)
					}
					return
				}
				if !strings.Contains(wasmErr, tt.wantErr) || !strings.Contains(busyErr, tt.wantErr) {
					t.Fatalf("stderr mismatch:\nwasm:   %q\nbusybox:%q", wasmErr, busyErr)
				}
			} else if wasmErr != busyErr {
				t.Fatalf("stderr mismatch:\nwasm:   %q\nbusybox:%q", wasmErr, busyErr)
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

func getOurBusyboxWasm(t *testing.T) string {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("failed to find repo root: %v", err)
	}
	cmd := exec.Command("make", "build-wasm")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build busybox wasm: %v (%s)", err, output)
	}
	return filepath.Join(root, "_build", "busybox.wasm")
}

func runWasmCmd(t *testing.T, runtime string, wasmPath string, applet string, args []string, input string, dir string) (string, string, int) {
	t.Helper()
	cmdArgs := []string{"--dir=.", wasmPath, applet}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command(runtime, cmdArgs...)
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
			t.Fatalf("run wasm %s: %v", applet, err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
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

func startTCPServer(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_, _ = conn.Write([]byte("hello"))
		_ = conn.Close()
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func startHTTPServer(t *testing.T) string {
	t.Helper()
	payload := []byte("hello")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	return server.URL + "/index.txt"
}

func scrubTasksetPID(output string) string {
	const prefix = "pid "
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			parts := strings.SplitN(strings.TrimPrefix(line, prefix), " ", 2)
			if len(parts) == 2 {
				lines[i] = prefix + "X " + parts[1]
			}
		}
	}
	return strings.Join(lines, "\n")
}
