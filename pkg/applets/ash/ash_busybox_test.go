package ash_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAshBusyboxSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("busybox ash suite skipped in short mode")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	testDir := filepath.Join(root, "testdata", "ash_test")
	runAll := filepath.Join(testDir, "run-all")
	if _, err := os.Stat(runAll); err != nil {
		t.Fatalf("missing run-all: %v", err)
	}

	repoRoot := filepath.Clean(filepath.Join(root, "..", "..", ".."))
	ashBin := filepath.Join(repoRoot, "_build", "busybox")
	if _, err := os.Stat(ashBin); err != nil {
		build := exec.Command("make", "build")
		build.Dir = repoRoot
		if err := build.Run(); err != nil {
			t.Fatalf("build busybox: %v", err)
		}
	}

	cmd := exec.Command("sh", "-c", "ln -sf \"$THIS_SH\" ash && ./run-all")
	cmd.Dir = testDir
	cmd.Env = append(os.Environ(), "PATH="+testDir+":"+os.Getenv("PATH"))
	cmd.Env = append(cmd.Env, "THIS_SH="+ashBin)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("busybox ash suite failed: %v", err)
	}
}
