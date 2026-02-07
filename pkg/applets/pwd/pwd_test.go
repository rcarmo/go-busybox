package pwd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/pwd"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestPwd(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "physical_default",
			Args:     []string{},
			WantCode: core.ExitSuccess,
			Check: func(t *testing.T, dir string) {
				if err := os.Chdir(dir); err != nil {
					t.Fatal(err)
				}
				out, _, code := testutil.CaptureAndRun(t, pwd.Run, []string{}, "")
				testutil.AssertExitCode(t, code, core.ExitSuccess)
				if filepath.Clean(strings.TrimSpace(out.String())) == "" {
					t.Fatalf("empty output")
				}
			},
		},
		{
			Name:     "logical_pwd",
			Args:     []string{"-L"},
			WantCode: core.ExitSuccess,
			Check: func(t *testing.T, dir string) {
				pwdEnv := filepath.Join(dir, "logical")
				if err := os.MkdirAll(pwdEnv, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.Chdir(pwdEnv); err != nil {
					t.Fatal(err)
				}
				if err := os.Setenv("PWD", pwdEnv); err != nil {
					t.Fatal(err)
				}
				out, _, code := testutil.CaptureAndRun(t, pwd.Run, []string{"-L"}, "")
				testutil.AssertExitCode(t, code, core.ExitSuccess)
				if filepath.Clean(strings.TrimSpace(out.String())) == "" {
					t.Fatalf("empty output")
				}
			},
		},
		{
			Name:     "testsuite_parity",
			Args:     []string{},
			WantCode: core.ExitSuccess,
			Check: func(t *testing.T, dir string) {
				if err := os.Chdir(dir); err != nil {
					t.Fatal(err)
				}
				path, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				execDir := filepath.Dir(path)
				if err := os.Chdir(execDir); err != nil {
					t.Fatal(err)
				}
				cmdPwd := filepath.Join(execDir, "pwd")
				if _, err := os.Stat(cmdPwd); err != nil {
					cmdPwd = "pwd"
				}
				cmd := testutil.Command(cmdPwd)
				cmd.Dir = execDir
				out, err := cmd.Output()
				if err != nil {
					t.Fatalf("external pwd: %v", err)
				}
				stdio, outBuf, _ := testutil.CaptureStdio("")
				code := pwd.Run(stdio, []string{})
				if code != core.ExitSuccess {
					t.Fatalf("pwd exit code = %d", code)
				}
				if strings.TrimSpace(outBuf.String()) != strings.TrimSpace(string(out)) {
					t.Fatalf("pwd mismatch: got %q want %q", strings.TrimSpace(outBuf.String()), strings.TrimSpace(string(out)))
				}
			},
		},
	}

	testutil.RunAppletTests(t, pwd.Run, tests)
}
