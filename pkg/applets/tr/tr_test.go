package tr_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/tr"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestTr(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "basic",
			Args:     []string{"a-z", "A-Z"},
			Input:    "hello\n",
			WantCode: core.ExitSuccess,
			WantOut:  "HELLO\n",
		},
		{
			Name:     "delete",
			Args:     []string{"-d", "aeiou"},
			Input:    "hello\n",
			WantCode: core.ExitSuccess,
			WantOut:  "hll\n",
		},
		{
			Name:     "squeeze",
			Args:     []string{"-s", "a", "a"},
			Input:    "aaab\n",
			WantCode: core.ExitSuccess,
			WantOut:  "ab\n",
		},
		{
			Name:     "complement_delete",
			Args:     []string{"-cd", "a"},
			Input:    "abca\n",
			WantCode: core.ExitSuccess,
			WantOut:  "aa",
		},
	}

	testutil.RunAppletTests(t, tr.Run, tests)
}
