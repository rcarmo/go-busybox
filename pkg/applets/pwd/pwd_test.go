package pwd_test

import (
"os"
"path/filepath"
"testing"

"github.com/rcarmo/busybox-wasm/pkg/applets/pwd"
"github.com/rcarmo/busybox-wasm/pkg/core"
"github.com/rcarmo/busybox-wasm/pkg/testutil"
)

func TestPwd(t *testing.T) {
tests := []testutil.AppletTestCase{
{
Name:     "physical_default",
Args:     []string{},
WantCode: core.ExitSuccess,
Check: func(t *testing.T, dir string) {
stdio, out, _ := testutil.CaptureStdioNoInput()
if err := os.Chdir(dir); err != nil {
t.Fatal(err)
}
code := pwd.Run(stdio, []string{})
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
stdio, out, _ := testutil.CaptureStdioNoInput()
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
code := pwd.Run(stdio, []string{"-L"})
testutil.AssertExitCode(t, code, core.ExitSuccess)
if filepath.Clean(out.String()) == "" {
t.Fatalf("empty output")
}
},
},
}

testutil.RunAppletTests(t, pwd.Run, tests)
}
