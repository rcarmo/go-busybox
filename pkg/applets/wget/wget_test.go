package wget_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/wget"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestWget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	t.Cleanup(server.Close)
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "basic",
			Args:     []string{server.URL},
			WantCode: core.ExitSuccess,
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileExists(t, dir+"/index.html")
			},
		},
		{
			Name:     "output_flag",
			Args:     []string{"-O", "out.txt", server.URL},
			WantCode: core.ExitSuccess,
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileExists(t, dir+"/out.txt")
			},
		},
	}
	testutil.RunAppletTests(t, wget.Run, tests)
}
