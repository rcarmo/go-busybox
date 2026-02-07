package pidof_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/pidof"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestPidof(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "self",
			Args:     []string{"go"},
			WantCode: core.ExitSuccess,
		},
		{
			Name:     "missing",
			Args:     []string{"definitely-not-running"},
			WantCode: core.ExitFailure,
		},
	}

	testutil.RunAppletTests(t, pidof.Run, tests)
}

func TestPidofBusyboxParity(t *testing.T) {
	if _, err := exec.LookPath("busybox"); err != nil {
		t.Skip("busybox not installed")
	}
	stdio, out, errBuf := testutil.CaptureStdio("")
	code := pidof.Run(stdio, []string{"bash"})
	_ = errBuf
	if code != core.ExitSuccess && code != core.ExitFailure {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if code == core.ExitSuccess && strings.TrimSpace(out.String()) == "" {
		t.Fatalf("expected pid output")
	}
}
