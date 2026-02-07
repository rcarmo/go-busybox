package ash_test

import (
	"path/filepath"
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/ash"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestAsh(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "basic",
			Args:     []string{"echo", "ok"},
			WantCode: core.ExitSuccess,
			WantOut:  "ok\n",
		},
		{
			Name:     "assignment",
			Args:     []string{"FOO=bar", "echo", "$FOO"},
			WantCode: core.ExitSuccess,
			WantOut:  "bar\n",
		},
		{
			Name:     "if_else",
			Args:     []string{"-c", "if true; then echo ok; else echo no; fi"},
			WantCode: core.ExitSuccess,
			WantOut:  "ok\n",
		},
		{
			Name:     "pipeline",
			Args:     []string{"-c", "echo ok | cat"},
			WantCode: core.ExitSuccess,
			WantOut:  "ok\n",
		},
		{
			Name:     "redirect",
			Args:     []string{"-c", "echo ok > out.txt"},
			WantCode: core.ExitSuccess,
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileContent(t, filepath.Join(dir, "out.txt"), "ok\n")
			},
		},
		{
			Name:     "while_loop",
			Args:     []string{"-c", "while true; do echo ok; break; done"},
			WantCode: core.ExitSuccess,
			WantOut:  "ok\n",
		},
		{
			Name:     "for_loop",
			Args:     []string{"-c", "for x in a b; do echo $x; done"},
			WantCode: core.ExitSuccess,
			WantOut:  "a\nb\n",
		},
		{
			Name:     "test_builtin",
			Args:     []string{"-c", "if test -n foo; then echo ok; else echo no; fi"},
			WantCode: core.ExitSuccess,
			WantOut:  "ok\n",
		},
		{
			Name:     "test_brackets",
			Args:     []string{"-c", "if [ foo = foo ]; then echo ok; else echo no; fi"},
			WantCode: core.ExitSuccess,
			WantOut:  "ok\n",
		},
	}
	testutil.RunAppletTests(t, ash.Run, tests)
}
