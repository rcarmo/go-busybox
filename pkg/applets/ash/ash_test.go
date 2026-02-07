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
		{
			Name:     "command_sub",
			Args:     []string{"-c", "echo $(echo hello)"},
			WantCode: core.ExitSuccess,
			WantOut:  "hello\n",
		},
		{
			Name:     "backtick_sub",
			Args:     []string{"-c", "echo `echo world`"},
			WantCode: core.ExitSuccess,
			WantOut:  "world\n",
		},
		{
			Name:     "export_var",
			Args:     []string{"-c", "export FOO=bar; echo $FOO"},
			WantCode: core.ExitSuccess,
			WantOut:  "bar\n",
		},
		{
			Name:     "unset_var",
			Args:     []string{"-c", "FOO=bar; unset FOO; echo ${FOO:-empty}"},
			WantCode: core.ExitSuccess,
			WantOut:  "empty\n",
		},
		{
			Name:     "param_default",
			Args:     []string{"-c", "echo ${UNSET:-default}"},
			WantCode: core.ExitSuccess,
			WantOut:  "default\n",
		},
		{
			Name:     "param_length",
			Args:     []string{"-c", "VAR=hello; echo ${#VAR}"},
			WantCode: core.ExitSuccess,
			WantOut:  "5\n",
		},
		{
			Name:     "case_esac",
			Args:     []string{"-c", "case foo in bar) echo no;; foo) echo yes;; esac"},
			WantCode: core.ExitSuccess,
			WantOut:  "yes\n",
		},
		{
			Name:     "case_wildcard",
			Args:     []string{"-c", "case anything in *) echo matched;; esac"},
			WantCode: core.ExitSuccess,
			WantOut:  "matched\n",
		},
		{
			Name:     "function_def",
			Args:     []string{"-c", "greet() { echo hello; }; greet"},
			WantCode: core.ExitSuccess,
			WantOut:  "hello\n",
		},
		{
			Name:     "test_file_exists",
			Args:     []string{"-c", "if [ -e /etc/passwd ]; then echo yes; fi"},
			WantCode: core.ExitSuccess,
			WantOut:  "yes\n",
		},
		{
			Name:     "colon_noop",
			Args:     []string{"-c", ":; echo ok"},
			WantCode: core.ExitSuccess,
			WantOut:  "ok\n",
		},
		{
			Name:     "eval_builtin",
			Args:     []string{"-c", "CMD=test; eval echo $CMD"},
			WantCode: core.ExitSuccess,
			WantOut:  "test\n",
		},
	}
	testutil.RunAppletTests(t, ash.Run, tests)
}
