package integration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBusyboxTestsuite(t *testing.T) {
	if testing.Short() {
		t.Skip("busybox testsuite skipped in short mode")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(root, "..", ".."))
	suiteRoot := filepath.Join(repoRoot, "pkg", "testdata")
	suiteDir := filepath.Join(suiteRoot, "busybox-testsuite")
	runTest := filepath.Join(suiteDir, "runtest")
	if _, err := os.Stat(runTest); err != nil {
		t.Fatalf("missing runtest: %v", err)
	}

	busybox := filepath.Join(repoRoot, "_build", "busybox")
	if _, err := os.Stat(busybox); err != nil {
		build := exec.Command("make", "build")
		build.Dir = repoRoot
		if err := build.Run(); err != nil {
			t.Fatalf("build busybox: %v", err)
		}
	}
	link := filepath.Join(suiteDir, "busybox")
	if _, err := os.Stat(link); err != nil {
		_ = os.Remove(link)
		if err := os.Symlink(busybox, link); err != nil {
			t.Fatalf("link busybox: %v", err)
		}
	}

	cmd := exec.Command("sh", "-c", "./runtest")
	cmd.Dir = suiteDir
	cmd.Env = append(os.Environ(), "bindir="+suiteDir, "tsdir="+suiteDir)
	cmd.Env = append(cmd.Env, "PATH="+suiteDir+":"+os.Getenv("PATH"))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("busybox testsuite failed: %v", err)
	}
}
