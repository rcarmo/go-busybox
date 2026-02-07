package sed_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/sed"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestSed(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "substitute",
			Args:     []string{"s/foo/bar/", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "bar\nbar\n",
			Files: map[string]string{
				"input.txt": "foo\nfoo\n",
			},
		},
		{
			Name:     "print_only",
			Args:     []string{"-n", "p", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "foo\n",
			Files: map[string]string{
				"input.txt": "foo\n",
			},
		},
		{
			Name:     "delete",
			Args:     []string{"d", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "",
			Files: map[string]string{
				"input.txt": "foo\n",
			},
		},
		{
			Name:     "append",
			Args:     []string{"a bar", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "foo\nbar\n",
			Files: map[string]string{
				"input.txt": "foo\n",
			},
		},
		{
			Name:     "insert",
			Args:     []string{"i bar", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "bar\nfoo\n",
			Files: map[string]string{
				"input.txt": "foo\n",
			},
		},
		{
			Name:     "change",
			Args:     []string{"c bar", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "bar\n",
			Files: map[string]string{
				"input.txt": "foo\n",
			},
		},
	}

	testutil.RunAppletTests(t, sed.Run, tests)
}
