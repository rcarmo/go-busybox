package head_test

import (
"strings"
"testing"

"github.com/rcarmo/go-busybox/pkg/applets/head"
"github.com/rcarmo/go-busybox/pkg/core"
"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestHead(t *testing.T) {
input := strings.Join([]string{
"line 1",
"line 2",
"line 3",
"line 4",
"line 5",
"line 6",
"line 7",
"line 8",
"line 9",
"line 10",
"line 11",
"line 12",
"",
}, "\n")

tests := []testutil.AppletTestCase{
{
Name:     "default_lines",
Args:     []string{"head.input"},
WantCode: core.ExitSuccess,
WantOut: strings.Join([]string{
"line 1",
"line 2",
"line 3",
"line 4",
"line 5",
"line 6",
"line 7",
"line 8",
"line 9",
"line 10",
"",
}, "\n"),
Files: map[string]string{
"head.input": input,
},
},
{
Name:     "limit_lines",
Args:     []string{"-n", "2", "head.input"},
WantCode: core.ExitSuccess,
WantOut:  "line 1\nline 2\n",
Files: map[string]string{
"head.input": input,
},
},
{
Name:     "negative_lines",
Args:     []string{"-n", "-9", "head.input"},
WantCode: core.ExitUsage,
WantErr:  "head:",
Files: map[string]string{
"head.input": input,
},
},
{
Name:     "byte_count",
Args:     []string{"-c", "4", "bytes.txt"},
WantCode: core.ExitSuccess,
WantOut:  "abcd",
Files: map[string]string{
"bytes.txt": "abcdef",
},
},
{
Name:     "multi_header",
Args:     []string{"file1.txt", "file2.txt"},
WantCode: core.ExitSuccess,
WantOut:  "==> file1.txt <==\na\nb\n\n==> file2.txt <==\nc\n",
Files: map[string]string{
"file1.txt": "a\nb\n",
"file2.txt": "c\n",
},
},
{
Name:     "quiet_mode",
Args:     []string{"-q", "file1.txt", "file2.txt"},
WantCode: core.ExitSuccess,
WantOut:  "a\nb\nc\n",
Files: map[string]string{
"file1.txt": "a\nb\n",
"file2.txt": "c\n",
},
},
{
Name:     "missing_file",
Args:     []string{"/missing"},
WantCode: core.ExitFailure,
WantErr:  "head:",
},
}

testutil.RunAppletTests(t, head.Run, tests)
}
