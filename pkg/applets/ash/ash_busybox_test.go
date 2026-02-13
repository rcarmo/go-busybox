package ash_test

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestAshBusyboxDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("busybox ash suite skipped in short mode")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	testRoot := filepath.Join(root, "testdata", "ash_test")
	configPath := filepath.Join(testRoot, ".config")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("missing .config: %v", err)
	}

	repoRoot := filepath.Clean(filepath.Join(root, "..", "..", ".."))
	ourBusybox := ensureBusybox(t, repoRoot)
	refBusybox := findReferenceBusybox(t, repoRoot)

	ensureHelperBinaries(t, testRoot)

	baseEnv := sanitizedEnv()
	configEnv := loadConfigEnv(t, configPath)

	modules := globModules(t, testRoot)
	for _, module := range modules {
		module := module
		t.Run(filepath.Base(module), func(t *testing.T) {
			tests := listTests(t, module)
			for _, testFile := range tests {
				testFile := testFile
				t.Run(filepath.Base(testFile), func(t *testing.T) {
					refOut, refCode := runAshTest(t, refBusybox, testFile, module, testRoot, baseEnv, configEnv)
					ourOut, ourCode := runAshTest(t, ourBusybox, testFile, module, testRoot, baseEnv, configEnv)
					if refCode != ourCode {
						t.Fatalf("exit code mismatch: busybox=%d ours=%d", refCode, ourCode)
					}
					if refOut != ourOut {
						t.Fatalf("output mismatch:\n%s", formatDiff(refOut, ourOut))
					}
				})
			}
		})
	}
}

func ensureBusybox(t *testing.T, repoRoot string) string {
	t.Helper()
	path := filepath.Join(repoRoot, "_build", "busybox")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	build := exec.Command("make", "build")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build busybox: %v\n%s", err, out)
	}
	return path
}

func findReferenceBusybox(t *testing.T, repoRoot string) string {
	t.Helper()
	if ref := os.Getenv("BUSYBOX_REFERENCE"); ref != "" {
		if info, err := os.Stat(ref); err == nil && !info.IsDir() {
			return ref
		}
		t.Fatalf("BUSYBOX_REFERENCE set but invalid: %s", ref)
	}
	candidate := filepath.Join(repoRoot, "busybox-reference", "busybox")
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate
	}
	t.Skip("busybox-reference/busybox not found; set BUSYBOX_REFERENCE")
	return ""
}

func ensureHelperBinaries(t *testing.T, testRoot string) {
	t.Helper()
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc not available; skipping busybox ash suite")
	}
	helpers := []string{"printenv", "recho", "zecho"}
	for _, helper := range helpers {
		binPath := filepath.Join(testRoot, helper)
		if info, err := os.Stat(binPath); err == nil && info.Mode()&0111 != 0 {
			continue
		}
		srcPath := filepath.Join(testRoot, helper+".c")
		cmd := exec.Command("gcc", "-O2", "-o", helper, filepath.Base(srcPath))
		cmd.Dir = testRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", helper, err, out)
		}
	}
}

func sanitizedEnv() map[string]string {
	m := envMap(os.Environ())
	for key := range m {
		if key == "LANG" || key == "LANGUAGE" || strings.HasPrefix(key, "LC_") {
			delete(m, key)
		}
	}
	return m
}

func loadConfigEnv(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	m := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		m[parts[0]] = parts[1]
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan config: %v", err)
	}
	return m
}

func globModules(t *testing.T, testRoot string) []string {
	t.Helper()
	candidates, err := filepath.Glob(filepath.Join(testRoot, "ash-*"))
	if err != nil {
		t.Fatalf("glob modules: %v", err)
	}
	modules := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			continue
		}
		modules = append(modules, candidate)
	}
	if len(modules) == 0 {
		t.Fatalf("no ash-* modules under %s", testRoot)
	}
	sort.Strings(modules)
	return modules
}

func listTests(t *testing.T, module string) []string {
	t.Helper()
	entries, err := os.ReadDir(module)
	if err != nil {
		t.Fatalf("read dir %s: %v", module, err)
	}
	var tests []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".tests") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.Mode()&0111 == 0 {
			continue
		}
		tests = append(tests, filepath.Join(module, name))
	}
	sort.Strings(tests)
	return tests
}

func runAshTest(t *testing.T, shellPath, testFile, workDir, testRoot string, baseEnv, configEnv map[string]string) (string, int) {
	t.Helper()
	linkDir := t.TempDir()
	linkPath := filepath.Join(linkDir, "ash")
	if err := os.Symlink(shellPath, linkPath); err != nil {
		if err := copyFile(shellPath, linkPath); err != nil {
			t.Fatalf("link ash: %v", err)
		}
	}
	env := buildEnv(baseEnv, configEnv, linkDir, testRoot, linkPath)
	cmd := exec.Command(linkPath, "./"+filepath.Base(testFile))
	cmd.Dir = workDir
	cmd.Env = env
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return filterOutput(output.String()), exitCode(err)
}

func buildEnv(baseEnv, configEnv map[string]string, linkDir, testRoot, thisSh string) []string {
	merged := map[string]string{}
	for k, v := range baseEnv {
		merged[k] = v
	}
	path := merged["PATH"]
	if path == "" {
		path = os.Getenv("PATH")
	}
	merged["PATH"] = strings.Join([]string{linkDir, testRoot, path}, string(os.PathListSeparator))
	merged["THIS_SH"] = thisSh
	for k, v := range configEnv {
		merged[k] = v
	}
	return flattenEnv(merged)
}

func filterOutput(out string) string {
	var buf strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "ash: using fallback suid method" {
			continue
		}
		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(line)
	}
	if buf.Len() > 0 && strings.HasSuffix(out, "\n") {
		buf.WriteByte('\n')
	}
	return buf.String()
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if ok := errors.As(err, &exitErr); ok {
		return exitErr.ExitCode()
	}
	return 1
}

func formatDiff(refOut, ourOut string) string {
	if refOut == ourOut {
		return ""
	}
	refLines := strings.Split(refOut, "\n")
	ourLines := strings.Split(ourOut, "\n")
	max := len(refLines)
	if len(ourLines) > max {
		max = len(ourLines)
	}
	for i := 0; i < max; i++ {
		var refLine string
		var ourLine string
		if i < len(refLines) {
			refLine = refLines[i]
		}
		if i < len(ourLines) {
			ourLine = ourLines[i]
		}
		if refLine != ourLine {
			return fmt.Sprintf("line %d:\n- busybox: %q\n+ ours:    %q", i+1, refLine, ourLine)
		}
	}
	return "outputs differ"
}

func envMap(env []string) map[string]string {
	m := map[string]string{}
	for _, entry := range env {
		if eq := strings.Index(entry, "="); eq > 0 {
			m[entry[:eq]] = entry[eq+1:]
		}
	}
	return m
}

func flattenEnv(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	entries := make([]string, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, key+"="+env[key])
	}
	return entries
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
