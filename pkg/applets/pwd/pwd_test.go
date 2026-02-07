package pwd_test

import (
	"os"
	"path/filepath"
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
				if filepath.Clean(out.String()) == "" {
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
				if filepath.Clean(out.String()) == "" {
					t.Fatalf("empty output")
				}
			},
		},
	}

	testutil.RunAppletTests(t, pwd.Run, tests)
}
