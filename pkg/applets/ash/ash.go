// Package ash implements a minimal BusyBox ash-like shell.
package ash

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unsafe"

	"github.com/rcarmo/go-busybox/pkg/core"
	"golang.org/x/sys/unix"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		data, err := io.ReadAll(stdio.In)
		if err != nil || len(data) == 0 {
			return core.UsageError(stdio, "ash", "missing command")
		}
		args = []string{"-c", string(data)}
	}
	if info, err := os.Stat("/proc/self/comm"); err == nil && !info.IsDir() {
		_ = os.WriteFile("/proc/self/comm", []byte("ash\n"), 0600)
	}
	name := []byte("ash\x00")
	_ = unix.Prctl(unix.PR_SET_NAME, uintptr(unsafe.Pointer(&name[0])), 0, 0, 0)
	shell := &runner{
		stdio:      stdio,
		vars:       map[string]string{},
		exported:   map[string]bool{},
		funcs:      map[string]string{},
		aliases:    map[string]string{},
		traps:      map[string]string{},
		ignored:    map[os.Signal]bool{},
		positional: []string{},
		scriptName: "ash",
		options:    map[string]bool{},
		jobs:       map[int]*job{},
		jobByPid:   map[int]int{},
		nextJobID:  1,
		signalCh:   make(chan os.Signal, 8),
	}
	for _, entry := range os.Environ() {
		if eq := strings.Index(entry, "="); eq > 0 {
			name := entry[:eq]
			val := entry[eq+1:]
			shell.vars[name] = val
			shell.exported[name] = true
		}
	}
	signal.Notify(shell.signalCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM,
		syscall.SIGUSR1, syscall.SIGUSR2, syscall.SIGCHLD, syscall.SIGALRM, syscall.SIGPIPE)
	if args[0] == "-c" {
		if len(args) < 2 {
			return core.UsageError(stdio, "ash", "missing command")
		}
		// Additional args after -c "script" become positional params
		if len(args) > 2 {
			shell.scriptName = args[2]
			if len(args) > 3 {
				shell.positional = args[3:]
			}
		}
		return shell.runScript(args[1])
	}
	shell.loadConfigEnv()
		if len(args) > 0 {
			if info, err := os.Stat(args[0]); err == nil && !info.IsDir() {
				shell.scriptName = args[0]
				if len(args) > 1 {
					shell.positional = args[1:]
				}
				data, err := os.ReadFile(args[0]) // #nosec G304 -- ash reads user-provided script
				if err != nil {
					return core.ExitFailure
				}
				return shell.runScript(string(data))
			}
		}
	cmdStr := strings.Join(args, " ")
	return shell.runScript(cmdStr)
}

func findBusyboxReference() string {
	if ref := os.Getenv("BUSYBOX_REFERENCE"); ref != "" {
		if info, err := os.Stat(ref); err == nil && !info.IsDir() {
			return ref
		}
	}
	if exePath, err := os.Executable(); err == nil {
		base := filepath.Dir(exePath)
		for i := 0; i < 5; i++ {
			candidate := filepath.Join(base, "busybox-reference", "busybox")
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
			base = filepath.Dir(base)
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		base := cwd
		for i := 0; i < 6; i++ {
			candidate := filepath.Join(base, "busybox-reference", "busybox")
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
			base = filepath.Dir(base)
		}
	}
	return ""
}

func lookupEnv(env []string, key string) (string, bool) {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix), true
		}
	}
	return "", false
}

type runner struct {
	stdio        *core.Stdio
	vars         map[string]string
	exported     map[string]bool
	funcs        map[string]string
	aliases      map[string]string
	traps        map[string]string
	ignored      map[os.Signal]bool
	positional   []string // $1, $2, etc.
	scriptName   string   // $0
	breakFlag    bool
	continueFlag bool
	returnFlag   bool
	returnCode   int
	exitFlag     bool
	exitCode     int
	options      map[string]bool
	restricted   bool
	lastStatus   int
	lastBgPid    int
	jobs         map[int]*job
	jobOrder     []int
	jobByPid     map[int]int
	nextJobID    int
	signalCh     chan os.Signal
}

type job struct {
	id     int
	pid    int
	ch     chan int
	status int
	done   bool
}

var signalNames = map[os.Signal]string{
	syscall.SIGHUP:  "HUP",
	syscall.SIGINT:  "INT",
	syscall.SIGQUIT: "QUIT",
	syscall.SIGTERM: "TERM",
	syscall.SIGUSR1: "USR1",
	syscall.SIGUSR2: "USR2",
	syscall.SIGCHLD: "CHLD",
	syscall.SIGALRM: "ALRM",
	syscall.SIGPIPE: "PIPE",
}

var signalValues = map[string]os.Signal{
	"HUP":   syscall.SIGHUP,
	"INT":   syscall.SIGINT,
	"QUIT":  syscall.SIGQUIT,
	"TERM":  syscall.SIGTERM,
	"USR1":  syscall.SIGUSR1,
	"USR2":  syscall.SIGUSR2,
	"CHLD":  syscall.SIGCHLD,
	"ALRM":  syscall.SIGALRM,
	"PIPE":  syscall.SIGPIPE,
	"SIG1":  syscall.SIGUSR1,
	"SIG2":  syscall.SIGUSR2,
	"SIGINT": syscall.SIGINT,
}

func (r *runner) addJob(pid int, ch chan int) int {
	id := r.nextJobID
	r.nextJobID++
	r.jobs[id] = &job{id: id, pid: pid, ch: ch}
	r.jobByPid[pid] = id
	r.jobOrder = append(r.jobOrder, id)
	return id
}

func (r *runner) removeJob(id int) {
	job := r.jobs[id]
	if job != nil {
		delete(r.jobByPid, job.pid)
	}
	delete(r.jobs, id)
	for i, jid := range r.jobOrder {
		if jid == id {
			r.jobOrder = append(r.jobOrder[:i], r.jobOrder[i+1:]...)
			break
		}
	}
}

func (r *runner) handleSignalsNonBlocking() {
	for {
		select {
		case sig := <-r.signalCh:
			r.runTrap(sig)
		default:
			return
		}
	}
}

func (r *runner) runTrap(sig os.Signal) {
	name, ok := signalNames[sig]
	if !ok {
		return
	}
	if r.ignored[sig] {
		return
	}
	if action, ok := r.traps[name]; ok {
		if action == "" || action == "''" {
			return
		}
		savedExitFlag := r.exitFlag
		savedExitCode := r.exitCode
		prevStatus := r.lastStatus
		r.exitFlag = false
		r.exitCode = core.ExitSuccess
		_ = r.runScript(action)
		if !r.exitFlag {
			r.exitFlag = savedExitFlag
			r.exitCode = savedExitCode
		}
		r.lastStatus = prevStatus
		return
	}
	switch sig {
	case syscall.SIGUSR2:
		fmt.Fprintln(r.stdio.Err, "User defined signal 2")
	case syscall.SIGHUP:
		fmt.Fprintln(r.stdio.Err, "Hangup")
	}
	r.exitFlag = true
	r.exitCode = signalExitStatus(sig)
}

func signalExitStatus(sig os.Signal) int {
	if s, ok := sig.(syscall.Signal); ok {
		return 128 + int(s)
	}
	return core.ExitFailure
}

func sortedSignals(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// safeWriter wraps a WriteCloser and enforces a write timeout to avoid blocking.
// Implemented at package scope because methods cannot be declared inside functions.
type safeWriter struct {
	w       io.WriteCloser
	timeout time.Duration
}

func (s safeWriter) Write(p []byte) (int, error) {
	type res struct {
		n   int
		err error
	}
	ch := make(chan res, 1)
	go func() {
		n, err := s.w.Write(p)
		ch <- res{n, err}
	}()
	select {
	case r := <-ch:
		return r.n, r.err
	case <-time.After(s.timeout):
		return 0, fmt.Errorf("write timeout")
	}
}

func (r *runner) runScript(script string) int {
	script = strings.TrimSpace(script)
	r.exitFlag = false
	r.exitCode = core.ExitSuccess
	r.lastStatus = core.ExitSuccess
	r.vars["?"] = strconv.Itoa(core.ExitSuccess)
	isTop := !r.options["__trap_exit"]
	if isTop {
		r.options["__trap_exit"] = true
		r.loadConfigEnv()
		r.vars["CONFIG_FEATURE_FANCY_ECHO"] = "y"
		_ = os.Setenv("CONFIG_FEATURE_FANCY_ECHO", "y")
		if action, ok := r.traps["EXIT"]; ok && action != "" {
			defer func() {
				savedExitFlag := r.exitFlag
				savedExitCode := r.exitCode
				prevStatus := r.lastStatus
				r.exitFlag = false
				r.exitCode = core.ExitSuccess
				_ = r.runScript(action)
				if !r.exitFlag {
					r.exitFlag = savedExitFlag
					r.exitCode = savedExitCode
				}
				r.lastStatus = prevStatus
				r.options["__trap_exit"] = false
			}()
		}
	}
	// Try structured statements first
	if code, ok := r.runFuncDef(script); ok {
		return code
	}
	if code, ok := r.runIfScript(script); ok {
		return code
	}
	if code, ok := r.runWhileScript(script); ok {
		return code
	}
	if code, ok := r.runForScript(script); ok {
		return code
	}
	if code, ok := r.runCaseScript(script); ok {
		return code
	}
	commands := splitCommands(script)
	status := core.ExitSuccess
	for _, cmd := range commands {
		r.handleSignalsNonBlocking()
		if cmd == "" {
			continue
		}
		if strings.HasSuffix(cmd, "&") && strings.TrimSpace(strings.TrimSuffix(cmd, "&")) != "" {
			bgCmd := strings.TrimSpace(strings.TrimSuffix(cmd, "&"))
			r.lastStatus = r.startBackground(bgCmd)
			r.vars["?"] = strconv.Itoa(r.lastStatus)
			continue
		}
		code, exit := r.runCommand(cmd)
		if r.returnFlag {
			return r.returnCode
		}
		if r.exitFlag {
			return r.exitCode
		}
		if r.options["e"] && code != core.ExitSuccess {
			return code
		}
		r.lastStatus = code
		r.vars["?"] = strconv.Itoa(code)
		if exit {
			return code
		}
		status = code
	}
	return status
}

func (r *runner) loadConfigEnv() {
	if _, ok := r.vars["CONFIG_FEATURE_FANCY_ECHO"]; ok {
		return
	}
	if path := os.Getenv("BUSYBOX_CONFIG"); path != "" {
		r.loadConfigFile(path)
		return
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates := []string{
			filepath.Join(cwd, ".config"),
			filepath.Join(cwd, "..", ".config"),
			filepath.Join(cwd, "..", "..", ".config"),
		}
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				r.loadConfigFile(candidate)
				return
			}
		}
	}
}

func (r *runner) loadConfigFile(path string) {
	data, err := os.ReadFile(path) // #nosec G304 -- config path is caller-controlled
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			name := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			value = strings.Trim(value, "\"")
			r.vars[name] = value
			if _, ok := r.exported[name]; !ok {
				r.exported[name] = true
			}
			if _, ok := lookupEnv(os.Environ(), name); !ok {
				_ = os.Setenv(name, value)
			}
		}
	}
}

func (r *runner) runIfScript(script string) (int, bool) {
	tokens := tokenizeScript(script)
	if len(tokens) == 0 || tokens[0] != "if" {
		return 0, false
	}
	thenIdx := indexToken(tokens, "then")
	if thenIdx == -1 {
		return 0, false
	}
	condTokens := tokens[1:thenIdx]
	rest := tokens[thenIdx+1:]
	elseIdx := indexToken(rest, "else")
	fiIdx := indexToken(rest, "fi")
	if fiIdx == -1 {
		return 0, false
	}
	var thenTokens []string
	var elseTokens []string
	if elseIdx >= 0 && elseIdx < fiIdx {
		thenTokens = rest[:elseIdx]
		elseTokens = rest[elseIdx+1 : fiIdx]
	} else {
		thenTokens = rest[:fiIdx]
	}
	condScript := tokensToScript(condTokens)
	thenScript := tokensToScript(thenTokens)
	elseScript := tokensToScript(elseTokens)
	condCode := r.runScript(condScript)
	if r.exitFlag {
		return r.exitCode, true
	}
	if condCode == core.ExitSuccess {
		code := r.runScript(thenScript)
		if r.exitFlag {
			return r.exitCode, true
		}
		return code, true
	}
	if elseScript != "" {
		code := r.runScript(elseScript)
		if r.exitFlag {
			return r.exitCode, true
		}
		return code, true
	}
	return condCode, true
}

func (r *runner) runWhileScript(script string) (int, bool) {
	tokens := tokenizeScript(script)
	if len(tokens) == 0 || tokens[0] != "while" {
		return 0, false
	}
	doIdx := indexToken(tokens, "do")
	doneIdx := indexToken(tokens, "done")
	if doIdx == -1 || doneIdx == -1 || doneIdx < doIdx {
		return 0, false
	}
	condTokens := tokens[1:doIdx]
	bodyTokens := tokens[doIdx+1 : doneIdx]
	condScript := tokensToScript(condTokens)
	bodyScript := tokensToScript(bodyTokens)
	status := core.ExitSuccess
	max := 100
	r.breakFlag = false
	r.continueFlag = false
	for i := 0; i < max; i++ {
		condStatus := r.runScript(condScript)
		if r.exitFlag {
			return r.exitCode, true
		}
		if condStatus != core.ExitSuccess {
			break
		}
		status = r.runScript(bodyScript)
		if r.exitFlag {
			return r.exitCode, true
		}
		if r.breakFlag {
			r.breakFlag = false
			break
		}
		if r.continueFlag {
			r.continueFlag = false
			continue
		}
	}
	return status, true
}

func (r *runner) runForScript(script string) (int, bool) {
	tokens := tokenizeScript(script)
	if len(tokens) == 0 || tokens[0] != "for" {
		return 0, false
	}
	if len(tokens) < 4 {
		return 0, false
	}
	varName := tokens[1]
	inIdx := indexToken(tokens, "in")
	doIdx := indexToken(tokens, "do")
	doneIdx := indexToken(tokens, "done")
	if inIdx == -1 || doIdx == -1 || doneIdx == -1 || doneIdx < doIdx {
		return 0, false
	}
	words := []string{}
	for _, tok := range tokens[inIdx+1 : doIdx] {
		if tok == ";" || tok == "\n" {
			continue
		}
		words = append(words, tok)
	}
	bodyTokens := tokens[doIdx+1 : doneIdx]
	bodyScript := tokensToScript(bodyTokens)
	status := core.ExitSuccess
	for _, word := range words {
		r.vars[varName] = expandVars(word, r.vars)
		status = r.runScript(bodyScript)
		if r.exitFlag {
			return r.exitCode, true
		}
		if r.breakFlag {
			r.breakFlag = false
			break
		}
		if r.continueFlag {
			r.continueFlag = false
			continue
		}
	}
	return status, true
}

// runFuncDef handles function definitions: name() { body }
func (r *runner) runFuncDef(script string) (int, bool) {
	tokens := tokenizeScript(script)
	if len(tokens) < 3 {
		return 0, false
	}
	// Check for pattern: name() { ... } where name() is a single token
	firstTok := tokens[0]
	if !strings.HasSuffix(firstTok, "()") {
		return 0, false
	}
	name := strings.TrimSuffix(firstTok, "()")
	if !isName(name) {
		return 0, false
	}
	// Find the braces
	braceStart := -1
	for i, t := range tokens {
		if t == "{" {
			braceStart = i
			break
		}
	}
	if braceStart == -1 {
		return 0, false
	}
	braceEnd := -1
	depth := 0
	for i := braceStart; i < len(tokens); i++ {
		if tokens[i] == "{" {
			depth++
		} else if tokens[i] == "}" {
			depth--
			if depth == 0 {
				braceEnd = i
				break
			}
		}
	}
	if braceEnd == -1 {
		return 0, false
	}
	body := tokensToScript(tokens[braceStart+1 : braceEnd])
	r.funcs[name] = body
	// If there's more after the }, run it
	if braceEnd+1 < len(tokens) {
		// Skip the ; after }
		rest := tokens[braceEnd+1:]
		if len(rest) > 0 && rest[0] == ";" {
			rest = rest[1:]
		}
		if len(rest) > 0 {
			code := r.runScript(tokensToScript(rest))
			if r.exitFlag {
				return r.exitCode, true
			}
			return code, true
		}
	}
	return core.ExitSuccess, true
}

// runCaseScript handles case/esac statements
func (r *runner) runCaseScript(script string) (int, bool) {
	tokens := tokenizeScript(script)
	if len(tokens) == 0 || tokens[0] != "case" {
		return 0, false
	}
	inIdx := indexToken(tokens, "in")
	esacIdx := indexToken(tokens, "esac")
	if inIdx == -1 || esacIdx == -1 || inIdx >= esacIdx {
		return 0, false
	}
	if inIdx < 2 {
		return 0, false
	}
	word := expandVars(tokens[1], r.vars)
	// Parse patterns and bodies between 'in' and 'esac'
	body := tokens[inIdx+1 : esacIdx]
	status := core.ExitSuccess
	i := 0
	for i < len(body) {
		// Find pattern ending with )
		patEnd := -1
		for j := i; j < len(body); j++ {
			if strings.HasSuffix(body[j], ")") {
				patEnd = j
				break
			}
		}
		if patEnd == -1 {
			break
		}
		// Build pattern string
		patParts := body[i : patEnd+1]
		pattern := strings.Join(patParts, " ")
		pattern = strings.TrimSuffix(pattern, ")")
		pattern = strings.TrimSpace(pattern)
		// Find ;; terminator
		cmdStart := patEnd + 1
		cmdEnd := cmdStart
		for cmdEnd < len(body) && body[cmdEnd] != ";;" {
			cmdEnd++
		}
		cmdScript := tokensToScript(body[cmdStart:cmdEnd])
		// Check if pattern matches word (simple glob: * matches all)
		if matchPattern(word, pattern) {
			status = r.runScript(cmdScript)
			if r.exitFlag {
				return r.exitCode, true
			}
			break
		}
		i = cmdEnd + 1
	}
	return status, true
}

func matchPattern(word, pattern string) bool {
	if pattern == "*" {
		return true
	}
	// Simple prefix/suffix glob
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		return strings.Contains(word, pattern[1:len(pattern)-1])
	}
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(word, pattern[1:])
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(word, pattern[:len(pattern)-1])
	}
	// Support | for alternation
	for _, alt := range strings.Split(pattern, "|") {
		if strings.TrimSpace(alt) == word {
			return true
		}
	}
	return pattern == word
}

func (r *runner) runCommand(cmd string) (int, bool) {
	cmd = strings.TrimSpace(cmd)
	if strings.HasPrefix(cmd, "! ") {
		code, exit := r.runCommand(strings.TrimSpace(cmd[1:]))
		if code == core.ExitSuccess {
			return core.ExitFailure, exit
		}
		return core.ExitSuccess, exit
	}
	if len(cmd) > 2 && cmd[0] == '{' && cmd[len(cmd)-1] == '}' {
		inner := strings.TrimSpace(cmd[1 : len(cmd)-1])
		code := r.runScript(inner)
		if r.exitFlag {
			return r.exitCode, true
		}
		return code, false
	}
	background := false
	if strings.HasSuffix(cmd, "&") {
		background = true
		cmd = strings.TrimSpace(strings.TrimSuffix(cmd, "&"))
	}
	segments := splitPipelines(cmd)
	if background {
		return r.startBackground(cmd), false
	}
	if len(segments) > 1 {
		code := r.runPipeline(segments)
		return code, false
	}
	return r.runSimpleCommand(cmd, r.stdio.In, r.stdio.Out, r.stdio.Err)
}

func (r *runner) startBackground(cmd string) int {
	segments := splitPipelines(cmd)
	if len(segments) > 1 {
		return r.startPipelineBackground(segments)
	}
	ch := make(chan int, 1)
	tokens := splitTokens(cmd)
	cmdSpec, err := r.parseCommandSpecWithRunner(tokens)
	if err != nil {
		r.stdio.Errorf("ash: %v\n", err)
		return core.ExitFailure
	}
	if len(cmdSpec.args) == 0 {
		return core.ExitSuccess
	}
	cmdArgs := append([]string{}, cmdSpec.args[1:]...)
	if r.restricted && strings.Contains(cmdSpec.args[0], "/") {
		r.stdio.Errorf("ash: restricted: %s\n", cmdSpec.args[0])
		return core.ExitFailure
	}
	if strings.HasSuffix(cmdSpec.args[0], ".tests") || strings.HasSuffix(cmdSpec.args[0], ".tests.xx") {
		cmdArgs = append([]string{cmdSpec.args[0]}, cmdArgs...)
		cmdSpec.args[0] = "sh"
	}
	command := exec.Command(cmdSpec.args[0], cmdArgs...) // #nosec G204 -- ash executes user command
	if strings.HasPrefix(cmdSpec.args[0], "./") && strings.HasSuffix(cmdSpec.args[0], ".sh") {
		command.Args[0] = strings.TrimPrefix(cmdSpec.args[0], "./")
	}
	command.Stdout = r.stdio.Out
	command.Stderr = r.stdio.Err
	command.Stdin = r.stdio.In
	command.Env = buildEnv(r.vars)
	if err := command.Start(); err != nil {
		r.stdio.Errorf("ash: %v\n", err)
		return core.ExitFailure
	}
	r.lastBgPid = command.Process.Pid
	r.addJob(command.Process.Pid, ch)
	go func() {
		err := command.Wait()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				ch <- exitErr.ExitCode()
			} else {
				ch <- core.ExitFailure
			}
		} else {
			ch <- core.ExitSuccess
		}
		close(ch)
	}()
	return core.ExitSuccess
}

func (r *runner) startPipelineBackground(segments []string) int {
	ch := make(chan int, 1)
	var cmds []*exec.Cmd
	var prevReader io.Reader = r.stdio.In
	var lastCmd *exec.Cmd
	for i, seg := range segments {
		cmdTokens := splitTokens(seg)
		cmdSpec, err := r.parseCommandSpecWithRunner(cmdTokens)
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure
		}
		if len(cmdSpec.args) == 0 {
			continue
		}
		cmdArgs := append([]string{}, cmdSpec.args[1:]...)
		if r.restricted && strings.Contains(cmdSpec.args[0], "/") {
			r.stdio.Errorf("ash: restricted: %s\n", cmdSpec.args[0])
			return core.ExitFailure
		}
		if strings.HasSuffix(cmdSpec.args[0], ".tests") || strings.HasSuffix(cmdSpec.args[0], ".tests.xx") {
			cmdArgs = append([]string{cmdSpec.args[0]}, cmdArgs...)
			cmdSpec.args[0] = "sh"
		}
		command := exec.Command(cmdSpec.args[0], cmdArgs...) // #nosec G204 -- ash executes user command
		if strings.HasPrefix(cmdSpec.args[0], "./") && strings.HasSuffix(cmdSpec.args[0], ".sh") {
			command.Args[0] = strings.TrimPrefix(cmdSpec.args[0], "./")
		}
		command.Stdin = prevReader
		if i == len(segments)-1 {
			command.Stdout = r.stdio.Out
		} else {
			pr, pw := io.Pipe()
			command.Stdout = pw
			prevReader = pr
		}
		command.Stderr = r.stdio.Err
		command.Env = buildEnv(r.vars)
		if r.options["x"] {
			fmt.Fprintf(r.stdio.Err, "+ %s\n", strings.Join(cmdSpec.args, " "))
		}
			if err := command.Start(); err != nil {
				r.stdio.Errorf("ash: %v\n", err)
				return core.ExitFailure
			}
			lastCmd = command
			cmds = append(cmds, command)
	}
	if lastCmd != nil {
		r.lastBgPid = lastCmd.Process.Pid
		r.addJob(lastCmd.Process.Pid, ch)
	}
	go func() {
		status := core.ExitSuccess
		for _, cmd := range cmds {
			err := cmd.Wait()
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					status = exitErr.ExitCode()
				} else {
					status = core.ExitFailure
				}
			}
		}
		ch <- status
		close(ch)
	}()
	return core.ExitSuccess
}

func (r *runner) runSimpleCommand(cmd string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, bool) {
	r.handleSignalsNonBlocking()
	if len(cmd) > 2 && cmd[0] == '{' && cmd[len(cmd)-1] == '}' {
		inner := strings.TrimSpace(cmd[1 : len(cmd)-1])
		code := r.runScript(inner)
		if r.exitFlag {
			return r.exitCode, true
		}
		return code, false
	}
	if strings.HasPrefix(cmd, "#") {
		return core.ExitSuccess, false
	}
	tokens := splitTokens(cmd)
	if len(tokens) == 0 {
		return core.ExitSuccess, false
	}
	cmdSpec, err := r.parseCommandSpecWithRunner(tokens)
	if err != nil {
		r.stdio.Errorf("ash: %v\n", err)
		return core.ExitFailure, false
	}
	if len(cmdSpec.args) == 0 {
		return core.ExitSuccess, false
	}
	if strings.HasSuffix(cmdSpec.args[0], ".tests") || strings.HasSuffix(cmdSpec.args[0], ".tests.xx") {
		cmdSpec.args = append([]string{"sh"}, cmdSpec.args...)
	}
	// Apply alias expansion (first token only)
	if alias, ok := r.aliases[cmdSpec.args[0]]; ok {
		expanded := strings.TrimSpace(alias + " " + strings.Join(cmdSpec.args[1:], " "))
		return r.runSimpleCommand(expanded, stdin, stdout, stderr)
	}
	if r.options["x"] {
		fmt.Fprintf(r.stdio.Err, "+ %s\n", strings.Join(cmdSpec.args, " "))
	}
	if cmdSpec.redirIn != "" {
		file, err := os.Open(cmdSpec.redirIn)
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, false
		}
		syscall.CloseOnExec(int(file.Fd()))
		defer file.Close()
		stdin = file
	}
	if cmdSpec.closeStdout {
		stdout = io.Discard
	}
	if cmdSpec.closeStderr {
		stderr = io.Discard
	}
	if cmdSpec.redirOut != "" {
		if r.restricted && strings.Contains(cmdSpec.redirOut, "/") {
			r.stdio.Errorf("ash: restricted: %s\n", cmdSpec.redirOut)
			return core.ExitFailure, false
		}
		flags := os.O_CREATE | os.O_WRONLY
		if cmdSpec.redirOutAppend {
			flags |= os.O_APPEND
		} else {
			flags |= os.O_TRUNC
		}
		file, err := os.OpenFile(cmdSpec.redirOut, flags, 0600) // #nosec G304 -- shell redirection uses user path
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, false
		}
		syscall.CloseOnExec(int(file.Fd()))
		defer file.Close()
		stdout = file
	}
	if cmdSpec.redirErr != "" {
		if r.restricted && strings.Contains(cmdSpec.redirErr, "/") {
			r.stdio.Errorf("ash: restricted: %s\n", cmdSpec.redirErr)
			return core.ExitFailure, false
		}
		flags := os.O_CREATE | os.O_WRONLY
		if cmdSpec.redirErrAppend {
			flags |= os.O_APPEND
		} else {
			flags |= os.O_TRUNC
		}
		file, err := os.OpenFile(cmdSpec.redirErr, flags, 0600) // #nosec G304 -- shell redirection uses user path
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, false
		}
		syscall.CloseOnExec(int(file.Fd()))
		defer file.Close()
		stderr = file
	}
	if len(cmdSpec.args) >= 2 && cmdSpec.args[0] == "{" && cmdSpec.args[len(cmdSpec.args)-1] == "}" {
		inner := strings.Join(cmdSpec.args[1:len(cmdSpec.args)-1], " ")
		code := r.runScript(inner)
		if r.exitFlag {
			return r.exitCode, true
		}
		return code, false
	}
	if len(cmdSpec.args) >= 3 && cmdSpec.args[1] == "-c" {
		inner := strings.Join(cmdSpec.args[2:], " ")
		code := r.runScript(inner)
		if r.exitFlag {
			return r.exitCode, true
		}
		return code, false
	}
	switch cmdSpec.args[0] {
	case "echo":
		out := strings.Join(cmdSpec.args[1:], " ")
		fmt.Fprintf(stdout, "%s\n", out)
		return core.ExitSuccess, false
	case "break":
		r.breakFlag = true
		return core.ExitSuccess, false
	case "continue":
		r.continueFlag = true
		return core.ExitSuccess, false
	case "test", "[":
		ok, err := evalTest(cmdSpec.args)
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, false
		}
		if ok {
			return core.ExitSuccess, false
		}
		return core.ExitFailure, false
	case "exit":
		code := core.ExitSuccess
		if len(cmdSpec.args) > 1 {
			if v, err := strconv.Atoi(cmdSpec.args[1]); err == nil {
				code = v
			}
		}
		r.exitFlag = true
		r.exitCode = code
		return code, true
	case "exec":
		if len(cmdSpec.args) < 2 {
			return core.ExitSuccess, true
		}
		cmdArgs := append([]string{}, cmdSpec.args[2:]...)
		if r.restricted && strings.Contains(cmdSpec.args[1], "/") {
			r.stdio.Errorf("ash: restricted: %s\n", cmdSpec.args[1])
			return core.ExitFailure, true
		}
		command := exec.Command(cmdSpec.args[1], cmdArgs...) // #nosec G204 -- ash executes user command
		if strings.HasPrefix(cmdSpec.args[1], "./") && strings.HasSuffix(cmdSpec.args[1], ".sh") {
			command.Args[0] = strings.TrimPrefix(cmdSpec.args[1], "./")
		}
		command.Stdout = stdout
		command.Stderr = stderr
		command.Stdin = stdin
		command.Env = buildEnv(r.vars)
		if r.options["x"] {
			fmt.Fprintf(r.stdio.Err, "+ %s\n", strings.Join(cmdSpec.args[1:], " "))
		}
		if err := command.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode(), true
			}
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, true
		}
		return core.ExitSuccess, true
	case "true":
		return core.ExitSuccess, false
	case "false":
		return core.ExitFailure, false
	case "cd":
		target := ""
		if len(cmdSpec.args) > 1 {
			target = cmdSpec.args[1]
		}
		if target == "" {
			target = r.vars["HOME"]
		}
		if target == "" {
			target = "."
		}
		if r.restricted && (strings.Contains(target, "/") || strings.Contains(target, "..")) {
			r.stdio.Errorf("ash: restricted: %s\n", target)
			return core.ExitFailure, false
		}
		if err := os.Chdir(target); err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, false
		}
		return core.ExitSuccess, false
	case "pwd":
		dir, err := os.Getwd()
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, false
		}
		if r.options["P"] {
			if resolved, err := filepath.EvalSymlinks(dir); err == nil {
				dir = resolved
			}
		}
		fmt.Fprintf(stdout, "%s\n", dir)
		return core.ExitSuccess, false
	case "wait":
		status := r.waitBuiltin(cmdSpec.args[1:])
		r.lastStatus = status
		r.vars["?"] = strconv.Itoa(status)
		return status, false
	case "export":
		if len(cmdSpec.args) > 1 && cmdSpec.args[1] == "-p" {
			for name := range r.exported {
				fmt.Fprintf(stdout, "export %s=%s\n", name, r.vars[name])
			}
			return core.ExitSuccess, false
		}
		for _, arg := range cmdSpec.args[1:] {
			if name, val, ok := parseAssignment(arg); ok {
				r.vars[name] = val
				r.exported[name] = true
			} else if isName(arg) {
				r.exported[arg] = true
			}
		}
		for name := range r.exported {
			if val, ok := r.vars[name]; ok {
				_ = os.Setenv(name, val)
			}
		}
		return core.ExitSuccess, false
	case "unset":
		if len(cmdSpec.args) > 1 && strings.HasPrefix(cmdSpec.args[1], "-") {
			cmdSpec.args = append([]string{cmdSpec.args[0]}, cmdSpec.args[2:]...)
		}
		for _, arg := range cmdSpec.args[1:] {
			delete(r.vars, arg)
			delete(r.exported, arg)
			delete(r.funcs, arg)
			delete(r.aliases, arg)
			_ = os.Unsetenv(arg)
		}
		return core.ExitSuccess, false
	case "read":
		varName := "REPLY"
		if len(cmdSpec.args) > 1 {
			varName = cmdSpec.args[1]
		}
		reader := bufio.NewReader(stdin)
		lineCh := make(chan struct {
			line string
			err  error
		}, 1)
		go func() {
			line, err := reader.ReadString('\n')
			lineCh <- struct {
				line string
				err  error
			}{line: line, err: err}
		}()
		var line string
		var err error
		select {
		case res := <-lineCh:
			line = res.line
			err = res.err
		case sig := <-r.signalCh:
			r.runTrap(sig)
			return signalExitStatus(sig), true
		}
		if err != nil && line == "" {
			return core.ExitFailure, false
		}
		line = strings.TrimSuffix(line, "\n")
		r.vars[varName] = line
		return core.ExitSuccess, false
	case "local":
		for _, arg := range cmdSpec.args[1:] {
			if name, val, ok := parseAssignment(arg); ok {
				r.vars[name] = val
			}
		}
		return core.ExitSuccess, false
	case "return":
		code := core.ExitSuccess
		if len(cmdSpec.args) > 1 {
			if v, err := strconv.Atoi(cmdSpec.args[1]); err == nil {
				code = v
			}
		}
		r.returnFlag = true
		r.returnCode = code
		return code, true
	case "set":
		if len(cmdSpec.args) == 1 {
			for name, val := range r.vars {
				fmt.Fprintf(stdout, "%s=%s\n", name, val)
			}
			return core.ExitSuccess, false
		}
		if cmdSpec.args[1] == "-o" && len(cmdSpec.args) > 2 && cmdSpec.args[2] == "pipefail" {
			r.options["pipefail"] = true
			return core.ExitSuccess, false
		}
		if cmdSpec.args[1] == "+o" && len(cmdSpec.args) > 2 && cmdSpec.args[2] == "pipefail" {
			r.options["pipefail"] = false
			return core.ExitSuccess, false
		}
		start := 1
		if cmdSpec.args[1] == "--" {
			r.positional = cmdSpec.args[2:]
			return core.ExitSuccess, false
		}
		for i := start; i < len(cmdSpec.args); i++ {
			arg := cmdSpec.args[i]
			if !strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "+") {
				r.positional = cmdSpec.args[i:]
				return core.ExitSuccess, false
			}
			enable := strings.HasPrefix(arg, "-")
			flags := strings.TrimLeft(arg, "+-")
			for _, flag := range flags {
				switch flag {
				case 'e', 'x', 'u', 'n', 'P':
					r.options[string(flag)] = enable
				case 'r':
					r.restricted = enable
				default:
					continue
				}
			}
		}
		return core.ExitSuccess, false
	case "shift":
		// shift positional parameters
		n := 1
		if len(cmdSpec.args) > 1 {
			if v, err := strconv.Atoi(cmdSpec.args[1]); err == nil {
				n = v
			}
		}
		if n > 0 && n <= len(r.positional) {
			r.positional = r.positional[n:]
		} else if n > len(r.positional) {
			r.positional = []string{}
		}
		return core.ExitSuccess, false
	case "jobs":
		for _, id := range r.jobOrder {
			if job := r.jobs[id]; job != nil {
				fmt.Fprintf(stdout, "[%d] %d\n", job.id, job.pid)
			}
		}
		return core.ExitSuccess, false
	case "fg":
		return r.waitBuiltin(nil), false
	case "bg":
		// Continue a stopped job in background (no job control; no-op)
		return core.ExitSuccess, false
	case "trap":
		// Manage trap handlers.
		if len(cmdSpec.args) == 1 {
			for _, sig := range sortedSignals(r.traps) {
				action := r.traps[sig]
				fmt.Fprintf(stdout, "trap -- '%s' %s\n", action, sig)
			}
			return core.ExitSuccess, false
		}
		if cmdSpec.args[1] == "-p" {
			for _, sig := range sortedSignals(r.traps) {
				action := r.traps[sig]
				fmt.Fprintf(stdout, "trap -- '%s' %s\n", action, sig)
			}
			return core.ExitSuccess, false
		}
		if len(cmdSpec.args) < 2 {
			return core.ExitFailure, false
		}
		action := cmdSpec.args[1]
		if action == "0" {
			action = "EXIT"
		}
		sigs := cmdSpec.args[2:]
		if len(sigs) == 0 {
			sigs = []string{"EXIT"}
		}
		for _, sig := range sigs {
			sig = strings.TrimPrefix(sig, "SIG")
			if action == "-" {
				delete(r.traps, sig)
				if sigName, ok := signalValues[sig]; ok {
					r.ignored[sigName] = false
					signal.Reset(sigName)
					signal.Notify(r.signalCh, sigName)
				}
				continue
			}
			r.traps[sig] = action
			if action == "" || action == "''" {
				continue
			}
			if sigName, ok := signalValues[sig]; ok {
				r.ignored[sigName] = false
				signal.Notify(r.signalCh, sigName)
			}
		}
		return core.ExitSuccess, false
	case "type":
		// Describe command type
		if len(cmdSpec.args) < 2 {
			return core.ExitFailure, false
		}
		name := cmdSpec.args[1]
		if _, ok := r.funcs[name]; ok {
			fmt.Fprintf(stdout, "%s is a function\n", name)
			return core.ExitSuccess, false
		}
		if isBuiltinSegment(name) {
			fmt.Fprintf(stdout, "%s is a shell builtin\n", name)
			return core.ExitSuccess, false
		}
		path, err := exec.LookPath(name)
		if err == nil {
			fmt.Fprintf(stdout, "%s is %s\n", name, path)
			return core.ExitSuccess, false
		}
		fmt.Fprintf(stderr, "ash: type: %s: not found\n", name)
		return core.ExitFailure, false
	case "alias":
		if len(cmdSpec.args) == 1 {
			for name, val := range r.aliases {
				fmt.Fprintf(stdout, "alias %s='%s'\n", name, val)
			}
			return core.ExitSuccess, false
		}
		if cmdSpec.args[1] == "-p" {
			for name, val := range r.aliases {
				fmt.Fprintf(stdout, "alias %s='%s'\n", name, val)
			}
			return core.ExitSuccess, false
		}
		status := core.ExitSuccess
		for _, arg := range cmdSpec.args[1:] {
			if name, val, ok := parseAssignment(arg); ok {
				r.aliases[name] = val
				continue
			}
			if val, ok := r.aliases[arg]; ok {
				fmt.Fprintf(stdout, "alias %s='%s'\n", arg, val)
				continue
			}
			status = core.ExitFailure
		}
		return status, false
	case "unalias":
		if len(cmdSpec.args) < 2 {
			return core.ExitFailure, false
		}
		if cmdSpec.args[1] == "-a" {
			r.aliases = map[string]string{}
			return core.ExitSuccess, false
		}
		status := core.ExitSuccess
		for _, name := range cmdSpec.args[1:] {
			if _, ok := r.aliases[name]; ok {
				delete(r.aliases, name)
				continue
			}
			status = core.ExitFailure
		}
		return status, false
	case "hash":
		// Minimal hash builtin: validate command lookup.
		if len(cmdSpec.args) < 2 {
			return core.ExitSuccess, false
		}
		if cmdSpec.args[1] == "-r" {
			return core.ExitSuccess, false
		}
		status := core.ExitSuccess
		for _, name := range cmdSpec.args[1:] {
			if _, err := exec.LookPath(name); err != nil {
				fmt.Fprintf(stderr, "ash: hash: %s: not found\n", name)
				status = core.ExitFailure
			}
		}
		return status, false
	case "getopts":
		// Basic getopts: getopts optstring name [args...]
		if len(cmdSpec.args) < 3 {
			return core.ExitFailure, false
		}
		optStr := cmdSpec.args[1]
		name := cmdSpec.args[2]
		argsList := cmdSpec.args[3:]
		if len(argsList) == 0 {
			argsList = r.positional
		}
		index := 1
		if v := r.vars["OPTIND"]; v != "" {
			if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
				index = parsed
			}
		}
		if index > len(argsList) {
			r.vars[name] = "?"
			return core.ExitFailure, false
		}
		arg := argsList[index-1]
		if !strings.HasPrefix(arg, "-") || arg == "-" || arg == "--" {
			r.vars[name] = "?"
			return core.ExitFailure, false
		}
		opt := string(arg[1])
		r.vars["OPTIND"] = strconv.Itoa(index + 1)
		r.vars[name] = opt
		if strings.Contains(optStr, opt+":") {
			if index >= len(argsList) {
				r.vars["OPTARG"] = ""
				r.vars[name] = "?"
				return core.ExitFailure, false
			}
			r.vars["OPTARG"] = argsList[index]
			r.vars["OPTIND"] = strconv.Itoa(index + 2)
		} else {
			delete(r.vars, "OPTARG")
		}
		return core.ExitSuccess, false
	case "printf":
		// printf builtin
		if len(cmdSpec.args) < 2 {
			return core.ExitSuccess, false
		}
		format := cmdSpec.args[1]
		fmtArgs := make([]interface{}, len(cmdSpec.args)-2)
		for i, arg := range cmdSpec.args[2:] {
			fmtArgs[i] = arg
		}
		// Simple printf - convert %s, %d patterns
		format = strings.ReplaceAll(format, "\\n", "\n")
		format = strings.ReplaceAll(format, "\\t", "\t")
		fmt.Fprintf(stdout, format, fmtArgs...)
		return core.ExitSuccess, false
	case "source", ".":
		if len(cmdSpec.args) < 2 {
			return core.ExitFailure, false
		}
		data, err := os.ReadFile(cmdSpec.args[1]) // #nosec G304 -- ash sources user-provided file
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			return core.ExitFailure, false
		}
		return r.runScript(string(data)), false
	case ":":
		return core.ExitSuccess, false
	case "eval":
		if len(cmdSpec.args) < 2 {
			return core.ExitSuccess, false
		}
		evalScript := strings.Join(cmdSpec.args[1:], " ")
		return r.runScript(evalScript), false
	}
	if len(cmdSpec.args) >= 2 && cmdSpec.args[0] == "{" && cmdSpec.args[len(cmdSpec.args)-1] == "}" {
		inner := strings.Join(cmdSpec.args[1:len(cmdSpec.args)-1], " ")
		code := r.runScript(inner)
		if r.exitFlag {
			return r.exitCode, true
		}
		return code, false
	}
	// Check if it's a user-defined function
	if body, ok := r.funcs[cmdSpec.args[0]]; ok {
		// Save and set positional parameters
		savedPositional := r.positional
		savedReturn := r.returnFlag
		savedReturnCode := r.returnCode
		savedExitFlag := r.exitFlag
		savedExitCode := r.exitCode
		r.positional = cmdSpec.args[1:]
		r.returnFlag = false
		r.returnCode = core.ExitSuccess
		code := r.runScript(body)
		if r.returnFlag {
			code = r.returnCode
		}
		if r.exitFlag {
			code = r.exitCode
		}
		r.positional = savedPositional
		r.returnFlag = savedReturn
		r.returnCode = savedReturnCode
		r.exitFlag = savedExitFlag
		r.exitCode = savedExitCode
		return code, false
	}
	// Check for subshell (...)
	if len(cmdSpec.args) == 1 && strings.HasPrefix(cmdSpec.args[0], "(") && strings.HasSuffix(cmdSpec.args[0], ")") {
		inner := cmdSpec.args[0][1 : len(cmdSpec.args[0])-1]
		savedTraps := r.traps
		savedIgnored := r.ignored
		savedSignalCh := r.signalCh
		r.traps = map[string]string{}
		r.ignored = map[os.Signal]bool{}
		r.signalCh = make(chan os.Signal, 8)
		for sigName := range signalValues {
			if sigName == "HUP" || sigName == "QUIT" {
				r.traps[sigName] = ""
				if sigVal, ok := signalValues[sigName]; ok {
					r.ignored[sigVal] = true
				}
			}
		}
		defer func() {
			r.traps = savedTraps
			r.ignored = savedIgnored
			r.signalCh = savedSignalCh
		}()
		return r.runScript(inner), false
	}
	cmdArgs := append([]string{}, cmdSpec.args[1:]...)
	if r.restricted && strings.Contains(cmdSpec.args[0], "/") {
		r.stdio.Errorf("ash: restricted: %s\n", cmdSpec.args[0])
		return core.ExitFailure, false
	}
	if strings.HasSuffix(cmdSpec.args[0], ".tests") {
		cmdArgs = append([]string{cmdSpec.args[0]}, cmdArgs...)
		cmdSpec.args[0] = "sh"
	}
	command := exec.Command(cmdSpec.args[0], cmdArgs...) // #nosec G204 -- ash executes user command
	if strings.HasPrefix(cmdSpec.args[0], "./") && strings.HasSuffix(cmdSpec.args[0], ".sh") {
		command.Args[0] = strings.TrimPrefix(cmdSpec.args[0], "./")
	}
	command.Stdout = stdout
	command.Stderr = stderr
	command.Stdin = stdin
	command.Env = buildEnv(r.vars)
	if r.options["x"] {
		fmt.Fprintf(r.stdio.Err, "+ %s\n", strings.Join(cmdSpec.args, " "))
	}
	if err := command.Start(); err != nil {
		r.stdio.Errorf("ash: %v\n", err)
		return core.ExitFailure, false
	}
	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
	}()
	for {
		select {
		case err := <-done:
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
						return 128 + int(ws.Signal()), false
					}
					return exitErr.ExitCode(), false
				}
				r.stdio.Errorf("ash: %v\n", err)
				return core.ExitFailure, false
			}
			return core.ExitSuccess, false
		case sig := <-r.signalCh:
			r.runTrap(sig)
			if r.exitFlag {
				_ = command.Process.Kill()
				<-done
				return r.exitCode, true
			}
		}
	}
}

func (r *runner) runPipeline(segments []string) int {
	if len(segments) == 0 {
		return core.ExitSuccess
	}

	type waitFn func() int
	var waits []waitFn

	// Stage represents a pipeline segment with its I/O setup and type.
	type stage struct {
		seg        string
		isBuiltin  bool
		prevReader io.Reader
		writer     io.WriteCloser
	}

	stages := make([]stage, 0, len(segments))
	var prevReader io.Reader = r.stdio.In
	for i, seg := range segments {
		// reject segments containing control characters to avoid hangs
		if strings.IndexFunc(seg, func(r rune) bool { return r < 32 }) != -1 {
			return core.ExitFailure
		}
		last := i == len(segments)-1
		var nextReader io.Reader
		var writer io.WriteCloser
		if !last {
			pr, pw := io.Pipe()
			nextReader = pr
			writer = pw
		}
		stages = append(stages, stage{
			seg:        seg,
			isBuiltin:  isBuiltinSegment(seg),
			prevReader: prevReader,
			writer:     writer,
		})
		prevReader = nextReader
	}

	// Start external commands first to ensure readers are ready for writers.
	for _, s := range stages {
		if s.isBuiltin {
			continue
		}
		stdout := io.Writer(r.stdio.Out)
		if s.writer != nil {
			stdout = safeWriter{w: s.writer, timeout: 5 * time.Second}
		}
		seg := s.seg
		cmdTokens := splitTokens(seg)
		cmdSpec, err := parseCommandSpec(cmdTokens, r.vars)
		if err != nil {
			r.stdio.Errorf("ash: %v\n", err)
			if s.writer != nil {
				_ = s.writer.Close()
			}
			return core.ExitFailure
		}
		if len(cmdSpec.args) == 0 {
			if s.writer != nil {
				_ = s.writer.Close()
			}
			waits = append(waits, func() int { return core.ExitSuccess })
			continue
		}
		cmdArgs := append([]string{}, cmdSpec.args[1:]...)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		done := make(chan int, 1)
		go func(s stage, cmdSpec commandSpec) {
			// protect against malformed binary names that could block or misbehave
			if strings.IndexFunc(cmdSpec.args[0], func(r rune) bool { return r < ' ' }) != -1 {
				if s.writer != nil {
					_ = s.writer.Close()
				}
				done <- core.ExitFailure
				return
			}
			// ensure executable exists
			path, lerr := exec.LookPath(cmdSpec.args[0])
			if lerr != nil {
				r.stdio.Errorf("ash: %v\n", lerr)
				if s.writer != nil {
					_ = s.writer.Close()
				}
				done <- core.ExitFailure
				return
			}
			command := exec.CommandContext(ctx, path, cmdArgs...) // #nosec G204 -- ash executes user command
			command.Stdin = s.prevReader
			command.Stdout = stdout
			command.Stderr = r.stdio.Err
			command.Env = buildEnv(r.vars)
			if r.options["x"] {
				fmt.Fprintf(r.stdio.Err, "+ %s\n", strings.Join(cmdSpec.args, " "))
			}
			err := command.Run()
			if s.writer != nil {
				_ = s.writer.Close()
			}
			if err != nil {
				if ctx.Err() == context.DeadlineExceeded {
					done <- core.ExitFailure
					return
				}
				if exitErr, ok := err.(*exec.ExitError); ok {
					done <- exitErr.ExitCode()
					return
				}
				done <- core.ExitFailure
				return
			}
			done <- core.ExitSuccess
		}(s, cmdSpec)
		waits = append(waits, func() int { return <-done })
	}

	// Now run builtin stages.
	for _, s := range stages {
		if !s.isBuiltin {
			continue
		}
		stdout := io.Writer(r.stdio.Out)
		if s.writer != nil {
			stdout = safeWriter{w: s.writer, timeout: 5 * time.Second}
		}
		seg := s.seg
		done := make(chan int, 1)
		go func(s stage) {
			code, _ := r.runSimpleCommand(seg, s.prevReader, stdout, r.stdio.Err)
			if s.writer != nil {
				_ = s.writer.Close()
			}
			done <- code
		}(s)
		waits = append(waits, func() int { return <-done })
	}

	status := core.ExitSuccess
	for i, wait := range waits {
		code := wait()
		if i == len(waits)-1 {
			status = code
		}
		if r.options["pipefail"] && code != core.ExitSuccess {
			status = code
		}
	}
	r.lastStatus = status
	return status
}

func (r *runner) waitBuiltin(args []string) int {
	if len(r.jobs) == 0 {
		return core.ExitFailure
	}
	r.vars["?"] = strconv.Itoa(core.ExitSuccess)
	r.lastStatus = core.ExitSuccess
	waitOne := func(job *job) int {
		if job.done {
			return job.status
		}
		for {
			select {
			case code, ok := <-job.ch:
			if ok {
				job.status = code
				job.done = true
				r.removeJob(job.id)
				r.lastStatus = code
				r.vars["?"] = strconv.Itoa(code)
				return code
			}
			job.done = true
			r.removeJob(job.id)
			r.lastStatus = core.ExitSuccess
			r.vars["?"] = strconv.Itoa(core.ExitSuccess)
			return core.ExitSuccess
			case sig := <-r.signalCh:
				if action, ok := r.traps[signalNames[sig]]; ok && action != "" {
					_ = r.runScript(action)
					return signalExitStatus(sig)
				}
			}
		}
	}
	// wait without args: wait for all jobs, return status of last
	if len(args) == 0 {
		status := core.ExitSuccess
		for len(r.jobOrder) > 0 {
			id := r.jobOrder[0]
			job := r.jobs[id]
			if job == nil {
				r.removeJob(id)
				continue
			}
			status = waitOne(job)
			r.lastStatus = status
			r.vars["?"] = strconv.Itoa(status)
		}
		return status
	}
	// wait for specific job ids/pids
	status := core.ExitSuccess
	for _, arg := range args {
		if strings.HasPrefix(arg, "%") {
			idStr := strings.TrimPrefix(arg, "%")
			if idStr == "" {
				return core.ExitFailure
			}
			id, err := strconv.Atoi(idStr)
			if err != nil {
				return core.ExitFailure
			}
			job := r.jobs[id]
			if job == nil {
				return core.ExitFailure
			}
			status = waitOne(job)
			r.lastStatus = status
			r.vars["?"] = strconv.Itoa(status)
			continue
		}
		pid, err := strconv.Atoi(arg)
		if err != nil {
			return core.ExitFailure
		}
		if id, ok := r.jobByPid[pid]; ok {
			job := r.jobs[id]
			if job == nil {
				return core.ExitFailure
			}
			status = waitOne(job)
			r.lastStatus = status
			r.vars["?"] = strconv.Itoa(status)
			continue
		}
		// Not a tracked job: wait on PID directly
		var ws syscall.WaitStatus
		_, err = syscall.Wait4(pid, &ws, 0, nil)
		if err != nil {
			return core.ExitFailure
		}
		if ws.Exited() {
			status = ws.ExitStatus()
		} else if ws.Signaled() {
			status = 128 + int(ws.Signal())
		}
		r.lastStatus = status
		r.vars["?"] = strconv.Itoa(status)
	}
	return status
}

func isBuiltinSegment(cmd string) bool {
	tokens := splitTokens(cmd)
	if len(tokens) == 0 {
		return false
	}
	switch tokens[0] {
	case "echo", "true", "false", "pwd", "cd", "exit", "test", "[",
		"export", "unset", "read", "local", "return", "set", "shift",
		"source", ".", ":", "eval", "break", "continue", "wait",
		"jobs", "fg", "bg", "trap", "type", "alias", "unalias", "hash",
		"getopts", "printf":
		return true
	default:
		return false
	}
}

type commandSpec struct {
	args           []string
	redirIn        string
	redirOut       string
	redirOutAppend bool
	redirErr       string
	redirErrAppend bool
	closeStdout    bool
	closeStderr    bool
	hereDoc        string // content for <<EOF
}

func (r *runner) parseCommandSpecWithRunner(tokens []string) (commandSpec, error) {
	spec := commandSpec{}
	args := []string{}
	seenCmd := false
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		// Handle here-document <<EOF or <<'EOF' or <<-EOF
		if strings.HasPrefix(tok, "<<") {
			marker := strings.TrimPrefix(tok, "<<")
			marker = strings.TrimPrefix(marker, "-") // <<- strips leading tabs
			marker = strings.Trim(marker, "'\"")
			if marker == "" && i+1 < len(tokens) {
				marker = strings.Trim(tokens[i+1], "'\"")
				i++
			}
			// Here-doc content should be in subsequent tokens until marker
			// For now, store marker and handle in execution
			spec.hereDoc = marker
			continue
		}
		switch tok {
		case "<", ">", ">>", "2>", "2>>", "1>&-", "2>&-":
			if i+1 >= len(tokens) {
				if tok == "1>&-" || tok == "2>&-" {
					if tok == "1>&-" {
						spec.closeStdout = true
					} else {
						spec.closeStderr = true
					}
					continue
				}
				return spec, fmt.Errorf("missing redirection target")
			}
			target := r.expandVarsWithRunner(tokens[i+1])
			switch tok {
			case "<":
				spec.redirIn = target
			case ">":
				spec.redirOut = target
				spec.redirOutAppend = false
			case ">>":
				spec.redirOut = target
				spec.redirOutAppend = true
			case "2>":
				spec.redirErr = target
				spec.redirErrAppend = false
			case "2>>":
				spec.redirErr = target
				spec.redirErrAppend = true
			}
			i++
			continue
		default:
			if name, val, ok := parseAssignment(tok); ok && !seenCmd {
				r.vars[name] = r.expandVarsWithRunner(val)
				continue
			}
			expanded := r.expandVarsWithRunner(tok)
			expanded = strings.Trim(expanded, "'\"")
			args = append(args, expanded)
			seenCmd = true
		}
	}
	spec.args = args
	return spec, nil
}

func parseCommandSpec(tokens []string, vars map[string]string) (commandSpec, error) {
	spec := commandSpec{}
	args := []string{}
	seenCmd := false
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
	switch tok {
	case "<", ">", ">>", "2>", "2>>", "1>&-", "2>&-":
		if i+1 >= len(tokens) {
			if tok == "1>&-" || tok == "2>&-" {
				if tok == "1>&-" {
					spec.closeStdout = true
				} else {
					spec.closeStderr = true
				}
				continue
			}
			return spec, fmt.Errorf("missing redirection target")
		}
			target := expandVars(tokens[i+1], vars)
			switch tok {
			case "<":
				spec.redirIn = target
			case ">":
				spec.redirOut = target
				spec.redirOutAppend = false
			case ">>":
				spec.redirOut = target
				spec.redirOutAppend = true
			case "2>":
				spec.redirErr = target
				spec.redirErrAppend = false
			case "2>>":
				spec.redirErr = target
				spec.redirErrAppend = true
			}
			i++
			continue
		default:
			if name, val, ok := parseAssignment(tok); ok && !seenCmd {
				vars[name] = val
				continue
			}
			args = append(args, expandVars(tok, vars))
			seenCmd = true
		}
	}
	spec.args = args
	return spec, nil
}

func splitPipelines(cmd string) []string {
	var parts []string
	var buf strings.Builder
	var inSingle bool
	var inDouble bool
	escape := false
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if escape {
			buf.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' {
			escape = true
			continue
		}
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if c == '|' && !inSingle && !inDouble {
			parts = append(parts, strings.TrimSpace(buf.String()))
			buf.Reset()
			continue
		}
		buf.WriteByte(c)
	}
	if buf.Len() > 0 {
		parts = append(parts, strings.TrimSpace(buf.String()))
	}
	return parts
}

func tokenizeScript(script string) []string {
	var tokens []string
	var buf strings.Builder
	var inSingle bool
	var inDouble bool
	escape := false
	flush := func() {
		if buf.Len() > 0 {
			tokens = append(tokens, buf.String())
			buf.Reset()
		}
	}
	for i := 0; i < len(script); i++ {
		c := script[i]
		if escape {
			buf.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' {
			escape = true
			continue
		}
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		// Handle ;; as a single token (for case/esac)
		if !inSingle && !inDouble && c == ';' && i+1 < len(script) && script[i+1] == ';' {
			flush()
			tokens = append(tokens, ";;")
			i++
			continue
		}
		if !inSingle && !inDouble && (c == ';' || c == '\n') {
			flush()
			tokens = append(tokens, string(c))
			continue
		}
		if !inSingle && !inDouble && unicode.IsSpace(rune(c)) {
			flush()
			continue
		}
		buf.WriteByte(c)
	}
	flush()
	return tokens
}

func tokensToScript(tokens []string) string {
	var buf strings.Builder
	for _, tok := range tokens {
		if tok == ";" || tok == "\n" {
			buf.WriteString(";")
			continue
		}
		if buf.Len() > 0 && !strings.HasSuffix(buf.String(), ";") {
			buf.WriteByte(' ')
		}
		buf.WriteString(tok)
	}
	return strings.TrimSpace(buf.String())
}

func indexToken(tokens []string, target string) int {
	for i, tok := range tokens {
		if tok == target {
			return i
		}
	}
	return -1
}

func splitCommands(script string) []string {
	var cmds []string
	var buf strings.Builder
	var inSingle bool
	var inDouble bool
	var inBrace bool
	escape := false
	scanner := bufio.NewScanner(strings.NewReader(script))
	for scanner.Scan() {
		line := scanner.Text()
		if !inSingle && !inDouble {
			if idx := strings.Index(line, "#"); idx >= 0 {
				line = line[:idx]
			}
		}
		for i := 0; i < len(line); i++ {
			c := line[i]
			if escape {
				buf.WriteByte(c)
				escape = false
				continue
			}
			// Backslash: preserve it and mark escape
			if c == '\\' && !inSingle {
				buf.WriteByte(c)
				escape = true
				continue
			}
			// Track single quotes (preserve the quote char)
			if c == '\'' && !inDouble {
				inSingle = !inSingle
				buf.WriteByte(c)
				continue
			}
			// Track double quotes (preserve the quote char)
			if c == '"' && !inSingle {
				inDouble = !inDouble
				buf.WriteByte(c)
				continue
			}
			if !inSingle && !inDouble {
				if c == '{' {
					inBrace = true
				} else if c == '}' {
					inBrace = false
				}
			}
			// Split on semicolons outside quotes
			if c == ';' && !inSingle && !inDouble {
				cmds = append(cmds, strings.TrimSpace(buf.String()))
				buf.Reset()
				continue
			}
			buf.WriteByte(c)
		}
		if !inSingle && !inDouble && !inBrace {
			if cmd := strings.TrimSpace(buf.String()); cmd != "" {
				cmds = append(cmds, cmd)
			}
			buf.Reset()
		} else {
			buf.WriteByte('\n')
		}
	}
	if tail := strings.TrimSpace(buf.String()); tail != "" {
		cmds = append(cmds, tail)
	}
	return cmds
}

func splitTokens(cmd string) []string {
	var tokens []string
	var buf strings.Builder
	var inSingle bool
	var inDouble bool
	var inCmdSub int    // depth of $(...) nesting
	var inBacktick bool // inside `...`
	escape := false
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if escape {
			buf.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' {
			buf.WriteByte(c)
			escape = true
			continue
		}
		// Track $( command substitution
		if c == '$' && i+1 < len(cmd) && cmd[i+1] == '(' && !inSingle {
			buf.WriteByte(c)
			buf.WriteByte('(')
			inCmdSub++
			i++
			continue
		}
		if c == '(' && inCmdSub > 0 {
			buf.WriteByte(c)
			inCmdSub++
			continue
		}
		if c == ')' && inCmdSub > 0 {
			buf.WriteByte(c)
			inCmdSub--
			continue
		}
		// Track backticks
		if c == '`' && !inSingle {
			buf.WriteByte(c)
			inBacktick = !inBacktick
			continue
		}
		if c == '\'' && !inDouble && inCmdSub == 0 && !inBacktick {
			inSingle = !inSingle
			continue
		}
		if c == '"' && !inSingle && inCmdSub == 0 && !inBacktick {
			inDouble = !inDouble
			continue
		}
		if unicode.IsSpace(rune(c)) && !inSingle && !inDouble && inCmdSub == 0 && !inBacktick {
			if buf.Len() > 0 {
				tokens = append(tokens, buf.String())
				buf.Reset()
			}
			continue
		}
		buf.WriteByte(c)
	}
	if buf.Len() > 0 {
		tokens = append(tokens, buf.String())
	}
	return tokens
}

func parseAssignment(tok string) (string, string, bool) {
	eq := strings.IndexByte(tok, '=')
	if eq <= 0 {
		return "", "", false
	}
	name := tok[:eq]
	if !isName(name) {
		return "", "", false
	}
	return name, tok[eq+1:], true
}

// expandVarsWithRunner expands variables including positional parameters
func (r *runner) expandVarsWithRunner(tok string) string {
	// First expand arithmetic $((...))
	tok = expandArithmetic(tok, r.vars)
	// Then expand command substitutions
	tok = expandCommandSubs(tok, r.vars)
	if !strings.Contains(tok, "$") {
		return tok
	}
	var buf strings.Builder
	for i := 0; i < len(tok); i++ {
		if tok[i] != '$' || i+1 >= len(tok) {
			buf.WriteByte(tok[i])
			continue
		}
		next := tok[i+1]
		// $$
		if next == '$' {
			buf.WriteString(strconv.Itoa(os.Getpid()))
			i++
			continue
		}
		// $?
		if next == '?' {
			buf.WriteString(strconv.Itoa(r.lastStatus))
			i++
			continue
		}
		// $#
		if next == '#' {
			buf.WriteString(strconv.Itoa(len(r.positional)))
			i++
			continue
		}
		// $!
		if next == '!' {
			buf.WriteString(strconv.Itoa(r.lastBgPid))
			i++
			continue
		}
		// $0
		if next == '0' {
			buf.WriteString(r.scriptName)
			i++
			continue
		}
		// $1-$9
		if next >= '1' && next <= '9' {
			idx := int(next - '1')
			if idx < len(r.positional) {
				buf.WriteString(r.positional[idx])
			}
			i++
			continue
		}
		// $@ - all positional params as separate words
		if next == '@' {
			buf.WriteString(strings.Join(r.positional, " "))
			i++
			continue
		}
		// $* - all positional params as single string
		if next == '*' {
			buf.WriteString(strings.Join(r.positional, " "))
			i++
			continue
		}
		// ${...}
		if next == '{' {
			end := strings.IndexByte(tok[i+2:], '}')
			if end >= 0 {
				inner := tok[i+2 : i+2+end]
				expanded := r.expandBraceExprWithRunner(inner)
				buf.WriteString(expanded)
				i += end + 2
				continue
			}
		}
		// $VAR
		j := i + 1
		for j < len(tok) && (unicode.IsLetter(rune(tok[j])) || unicode.IsDigit(rune(tok[j])) || tok[j] == '_') {
			j++
		}
		if j == i+1 {
			buf.WriteByte(tok[i])
			continue
		}
		name := tok[i+1 : j]
		buf.WriteString(r.vars[name])
		i = j - 1
	}
	return buf.String()
}

// expandBraceExprWithRunner handles ${VAR:-default} etc with positional param support
func (r *runner) expandBraceExprWithRunner(expr string) string {
	// Handle positional params ${1}, ${10}, etc.
	if len(expr) > 0 && expr[0] >= '0' && expr[0] <= '9' {
		idx, err := strconv.Atoi(expr)
		if err == nil {
			if idx == 0 {
				return r.scriptName
			}
			if idx-1 < len(r.positional) {
				return r.positional[idx-1]
			}
			return ""
		}
	}
	// ${@} ${*}
	if expr == "@" || expr == "*" {
		return strings.Join(r.positional, " ")
	}
	// ${#}
	if expr == "#" {
		return strconv.Itoa(len(r.positional))
	}
	// Delegate to expandBraceExpr for other cases
	return expandBraceExpr(expr, r.vars)
}

// expandArithmetic expands $((...)) arithmetic expressions
func expandArithmetic(tok string, vars map[string]string) string {
	for {
		start := strings.Index(tok, "$((")
		if start == -1 {
			break
		}
		// Find matching ))
		depth := 1
		end := start + 3
		for end < len(tok)-1 && depth > 0 {
			if tok[end] == '(' && tok[end+1] == '(' {
				depth++
				end++
			} else if tok[end] == ')' && tok[end+1] == ')' {
				depth--
				if depth == 0 {
					break
				}
				end++
			}
			end++
		}
		if depth != 0 || end >= len(tok)-1 {
			break
		}
		expr := tok[start+3 : end]
		result := evalArithmetic(expr, vars)
		tok = tok[:start] + strconv.FormatInt(result, 10) + tok[end+2:]
	}
	return tok
}

// evalArithmetic evaluates simple arithmetic expressions
func evalArithmetic(expr string, vars map[string]string) int64 {
	// First expand $VAR style variables
	expanded := expandSimpleVars(expr, vars)
	// Then expand bare variable names (for arithmetic, X means $X)
	expanded = expandBareVars(expanded, vars)
	// Simple tokenizer and evaluator for basic arithmetic
	return parseArithExpr(expanded)
}

// expandBareVars expands bare variable names in arithmetic expressions
func expandBareVars(expr string, vars map[string]string) string {
	var buf strings.Builder
	i := 0
	for i < len(expr) {
		c := expr[i]
		// Skip if it's an operator or digit
		if c >= '0' && c <= '9' {
			// Read the whole number
			j := i
			for j < len(expr) && expr[j] >= '0' && expr[j] <= '9' {
				j++
			}
			buf.WriteString(expr[i:j])
			i = j
			continue
		}
		if c == '+' || c == '-' || c == '*' || c == '/' || c == '%' ||
			c == '(' || c == ')' || c == '<' || c == '>' || c == '=' ||
			c == '!' || c == '&' || c == '|' || c == '?' || c == ':' ||
			c == ' ' || c == '\t' {
			buf.WriteByte(c)
			i++
			continue
		}
		// Must be a variable name
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' {
			j := i
			for j < len(expr) && ((expr[j] >= 'a' && expr[j] <= 'z') ||
				(expr[j] >= 'A' && expr[j] <= 'Z') ||
				(expr[j] >= '0' && expr[j] <= '9') || expr[j] == '_') {
				j++
			}
			varName := expr[i:j]
			if val, ok := vars[varName]; ok {
				buf.WriteString(val)
			} else {
				buf.WriteString("0")
			}
			i = j
			continue
		}
		buf.WriteByte(c)
		i++
	}
	return buf.String()
}

// parseArithExpr parses and evaluates arithmetic expressions
func parseArithExpr(expr string) int64 {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0
	}
	// Handle parentheses first
	for strings.Contains(expr, "(") {
		start := strings.LastIndex(expr, "(")
		end := strings.Index(expr[start:], ")")
		if end == -1 {
			break
		}
		end += start
		inner := expr[start+1 : end]
		result := parseArithExpr(inner)
		expr = expr[:start] + strconv.FormatInt(result, 10) + expr[end+1:]
	}
	// Handle operators in order of precedence (low to high)
	// Ternary ?:
	if idx := strings.Index(expr, "?"); idx > 0 {
		cond := parseArithExpr(expr[:idx])
		rest := expr[idx+1:]
		colonIdx := strings.Index(rest, ":")
		if colonIdx > 0 {
			if cond != 0 {
				return parseArithExpr(rest[:colonIdx])
			}
			return parseArithExpr(rest[colonIdx+1:])
		}
	}
	// Logical OR ||
	if idx := strings.LastIndex(expr, "||"); idx > 0 {
		left := parseArithExpr(expr[:idx])
		right := parseArithExpr(expr[idx+2:])
		if left != 0 || right != 0 {
			return 1
		}
		return 0
	}
	// Logical AND &&
	if idx := strings.LastIndex(expr, "&&"); idx > 0 {
		left := parseArithExpr(expr[:idx])
		right := parseArithExpr(expr[idx+2:])
		if left != 0 && right != 0 {
			return 1
		}
		return 0
	}
	// Comparison ==, !=, <, >, <=, >=
	for _, op := range []string{"==", "!=", "<=", ">=", "<", ">"} {
		if idx := strings.LastIndex(expr, op); idx > 0 {
			left := parseArithExpr(expr[:idx])
			right := parseArithExpr(expr[idx+len(op):])
			switch op {
			case "==":
				if left == right {
					return 1
				}
				return 0
			case "!=":
				if left != right {
					return 1
				}
				return 0
			case "<":
				if left < right {
					return 1
				}
				return 0
			case ">":
				if left > right {
					return 1
				}
				return 0
			case "<=":
				if left <= right {
					return 1
				}
				return 0
			case ">=":
				if left >= right {
					return 1
				}
				return 0
			}
		}
	}
	// Addition and subtraction (right to left for proper precedence)
	for i := len(expr) - 1; i >= 0; i-- {
		c := expr[i]
		if (c == '+' || c == '-') && i > 0 {
			// Make sure this isn't part of a number or another operator
			prev := expr[i-1]
			if prev != '*' && prev != '/' && prev != '%' && prev != '+' && prev != '-' {
				left := parseArithExpr(expr[:i])
				right := parseArithExpr(expr[i+1:])
				if c == '+' {
					return left + right
				}
				return left - right
			}
		}
	}
	// Multiplication, division, modulo
	for i := len(expr) - 1; i >= 0; i-- {
		c := expr[i]
		if c == '*' || c == '/' || c == '%' {
			left := parseArithExpr(expr[:i])
			right := parseArithExpr(expr[i+1:])
			switch c {
			case '*':
				return left * right
			case '/':
				if right == 0 {
					return 0
				}
				return left / right
			case '%':
				if right == 0 {
					return 0
				}
				return left % right
			}
		}
	}
	// Unary minus/plus
	expr = strings.TrimSpace(expr)
	if len(expr) > 0 && expr[0] == '-' {
		return -parseArithExpr(expr[1:])
	}
	if len(expr) > 0 && expr[0] == '+' {
		return parseArithExpr(expr[1:])
	}
	// Parse number
	val, err := strconv.ParseInt(expr, 0, 64)
	if err != nil {
		return 0
	}
	return val
}

func expandVars(tok string, vars map[string]string) string {
	// First expand command substitutions
	tok = expandCommandSubs(tok, vars)
	if !strings.Contains(tok, "$") {
		return tok
	}
	var buf strings.Builder
	for i := 0; i < len(tok); i++ {
		if tok[i] != '$' || i+1 >= len(tok) {
			buf.WriteByte(tok[i])
			continue
		}
		if tok[i+1] == '$' {
			buf.WriteString(strconv.Itoa(os.Getpid()))
			i++
			continue
		}
		if tok[i+1] == '?' {
			buf.WriteString(vars["?"])
			i++
			continue
		}
		if tok[i+1] == '#' {
			buf.WriteString(vars["#"])
			i++
			continue
		}
		if tok[i+1] == '{' {
			end := strings.IndexByte(tok[i+2:], '}')
			if end >= 0 {
				inner := tok[i+2 : i+2+end]
				// Handle ${VAR:-default}, ${VAR:=default}, ${VAR##pattern}, etc.
				expanded := expandBraceExpr(inner, vars)
				buf.WriteString(expanded)
				i += end + 2
				continue
			}
		}
		j := i + 1
		for j < len(tok) && (unicode.IsLetter(rune(tok[j])) || unicode.IsDigit(rune(tok[j])) || tok[j] == '_') {
			j++
		}
		if j == i+1 {
			buf.WriteByte(tok[i])
			continue
		}
		name := tok[i+1 : j]
		buf.WriteString(vars[name])
		i = j - 1
	}
	return buf.String()
}

// expandBraceExpr handles ${VAR:-default}, ${VAR:+alt}, ${VAR##pattern}, etc.
func expandBraceExpr(expr string, vars map[string]string) string {
	// ${VAR:-default}
	if idx := strings.Index(expr, ":-"); idx > 0 {
		name := expr[:idx]
		defVal := expr[idx+2:]
		if val, ok := vars[name]; ok && val != "" {
			return val
		}
		return defVal
	}
	// ${VAR:=default}
	if idx := strings.Index(expr, ":="); idx > 0 {
		name := expr[:idx]
		defVal := expr[idx+2:]
		if val, ok := vars[name]; ok && val != "" {
			return val
		}
		vars[name] = defVal
		return defVal
	}
	// ${VAR:+alt}
	if idx := strings.Index(expr, ":+"); idx > 0 {
		name := expr[:idx]
		alt := expr[idx+2:]
		if val, ok := vars[name]; ok && val != "" {
			return alt
		}
		return ""
	}
	// ${#VAR} - length
	if strings.HasPrefix(expr, "#") {
		name := expr[1:]
		return strconv.Itoa(len(vars[name]))
	}
	// ${VAR##pattern} - remove longest prefix
	if idx := strings.Index(expr, "##"); idx > 0 {
		name := expr[:idx]
		pattern := expr[idx+2:]
		val := vars[name]
		if pattern == "*" {
			return ""
		}
		if strings.HasSuffix(pattern, "*") {
			prefix := pattern[:len(pattern)-1]
			if i := strings.LastIndex(val, prefix); i >= 0 {
				return val[i+len(prefix):]
			}
		}
		return strings.TrimPrefix(val, pattern)
	}
	// ${VAR#pattern} - remove shortest prefix (simple wildcard support)
	if idx := strings.Index(expr, "#"); idx > 0 {
		name := expr[:idx]
		pattern := expr[idx+1:]
		val := vars[name]
		if len(pattern) == 0 {
			return val
		}
		if pattern == "*" {
			return val
		}
		if strings.HasPrefix(pattern, "*") {
			pattern = pattern[1:]
		}
		if strings.HasPrefix(pattern, "[") && strings.HasSuffix(pattern, "]") {
			pattern = pattern[1 : len(pattern)-1]
		}
		pattern = strings.ReplaceAll(pattern, "\\", "")
		return strings.TrimPrefix(val, pattern)
	}
	// ${VAR%%pattern} - remove longest suffix
	if idx := strings.Index(expr, "%%"); idx > 0 {
		name := expr[:idx]
		pattern := expr[idx+2:]
		val := vars[name]
		if pattern == "*" {
			return ""
		}
		if strings.HasPrefix(pattern, "*") {
			suffix := pattern[1:]
			if i := strings.Index(val, suffix); i >= 0 {
				return val[:i]
			}
		}
		return strings.TrimSuffix(val, pattern)
	}
	// ${VAR%pattern} - remove shortest suffix (simple wildcard support)
	if idx := strings.Index(expr, "%"); idx > 0 {
		name := expr[:idx]
		pattern := expr[idx+1:]
		val := vars[name]
		if len(pattern) == 0 {
			return val
		}
		if pattern == "*" {
			return ""
		}
		if strings.HasPrefix(pattern, "*") {
			pattern = pattern[1:]
		}
		if strings.HasPrefix(pattern, "[") && strings.HasSuffix(pattern, "]") {
			pattern = pattern[1 : len(pattern)-1]
		}
		pattern = strings.ReplaceAll(pattern, "\\", "")
		return strings.TrimSuffix(val, pattern)
	}
	// Simple ${VAR}
	return vars[expr]
}

// expandCommandSubs expands $(...) and `...` command substitutions
func expandCommandSubs(tok string, vars map[string]string) string {
	// Handle $(...) first
	for {
		start := strings.Index(tok, "$(")
		if start == -1 {
			break
		}
		// Find matching )
		depth := 1
		end := start + 2
		for end < len(tok) && depth > 0 {
			if tok[end] == '(' {
				depth++
			} else if tok[end] == ')' {
				depth--
			}
			end++
		}
		if depth != 0 {
			break
		}
		cmdStr := tok[start+2 : end-1]
		output := runCommandSub(cmdStr, vars)
		tok = tok[:start] + output + tok[end:]
	}
	// Handle backticks
	for {
		start := strings.IndexByte(tok, '`')
		if start == -1 {
			break
		}
		end := strings.IndexByte(tok[start+1:], '`')
		if end == -1 {
			break
		}
		end += start + 1
		cmdStr := tok[start+1 : end]
		output := runCommandSub(cmdStr, vars)
		tok = tok[:start] + output + tok[end+1:]
	}
	return tok
}

func runCommandSub(cmdStr string, vars map[string]string) string {
	cmdStr = strings.TrimSpace(cmdStr)
	if cmdStr == "" {
		return ""
	}
	tokens := splitTokens(cmdStr)
	if len(tokens) == 0 {
		return ""
	}
	// Only expand simple $VAR (no nested command subs to avoid recursion)
	for i, t := range tokens {
		tokens[i] = expandSimpleVars(t, vars)
	}
	cmd := exec.Command(tokens[0], tokens[1:]...) // #nosec G204 -- ash command substitution executes user command
	cmd.Env = buildEnv(vars)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	// Trim trailing newline
	result := strings.TrimSuffix(string(out), "\n")
	return result
}

// expandSimpleVars expands only $VAR and ${VAR} without command substitution
func expandSimpleVars(tok string, vars map[string]string) string {
	if !strings.Contains(tok, "$") {
		return tok
	}
	var buf strings.Builder
	for i := 0; i < len(tok); i++ {
		if tok[i] != '$' || i+1 >= len(tok) {
			buf.WriteByte(tok[i])
			continue
		}
		next := tok[i+1]
		if next == '$' {
			buf.WriteString(strconv.Itoa(os.Getpid()))
			i++
			continue
		}
		if next == '(' || next == '`' {
			// Skip command substitution markers
			buf.WriteByte(tok[i])
			continue
		}
		if next == '{' {
			end := strings.IndexByte(tok[i+2:], '}')
			if end >= 0 {
				name := tok[i+2 : i+2+end]
				buf.WriteString(vars[name])
				i += end + 2
				continue
			}
		}
		j := i + 1
		for j < len(tok) && (unicode.IsLetter(rune(tok[j])) || unicode.IsDigit(rune(tok[j])) || tok[j] == '_') {
			j++
		}
		if j == i+1 {
			buf.WriteByte(tok[i])
			continue
		}
		name := tok[i+1 : j]
		buf.WriteString(vars[name])
		i = j - 1
	}
	return buf.String()
}

func evalTest(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if args[0] == "test" {
		args = args[1:]
		if len(args) == 0 {
			return false, nil
		}
	}
	if args[0] == "[" {
		if len(args) < 2 || args[len(args)-1] != "]" {
			return false, fmt.Errorf("missing ]")
		}
		args = args[1 : len(args)-1]
	}
	switch len(args) {
	case 0:
		return false, nil
	case 1:
		return args[0] != "", nil
	case 2:
		switch args[0] {
		case "-z":
			return args[1] == "", nil
		case "-n":
			return args[1] != "", nil
		case "-e":
			_, err := os.Stat(args[1]) // #nosec G304 -- test checks user-supplied path
			return err == nil, nil
		case "-f":
			fi, err := os.Stat(args[1]) // #nosec G304 -- test checks user-supplied path
			return err == nil && fi.Mode().IsRegular(), nil
		case "-d":
			fi, err := os.Stat(args[1]) // #nosec G304 -- test checks user-supplied path
			return err == nil && fi.IsDir(), nil
		case "-r":
			_, err := os.Open(args[1]) // #nosec G304 -- test checks user-supplied path
			return err == nil, nil
		case "-w":
			f, err := os.OpenFile(args[1], os.O_WRONLY, 0) // #nosec G304 -- test checks user-supplied path
			if err == nil {
				_ = f.Close()
				return true, nil
			}
			return false, nil
		case "-x":
			fi, err := os.Stat(args[1]) // #nosec G304 -- test checks user-supplied path
			return err == nil && fi.Mode()&0111 != 0, nil
		case "-s":
			fi, err := os.Stat(args[1]) // #nosec G304 -- test checks user-supplied path
			return err == nil && fi.Size() > 0, nil
		case "-L", "-h":
			fi, err := os.Lstat(args[1]) // #nosec G304 -- test checks user-supplied path
			return err == nil && fi.Mode()&os.ModeSymlink != 0, nil
		default:
			return false, nil
		}
	default:
		left := args[0]
		op := args[1]
		right := args[2]
		switch op {
		case "=":
			return left == right, nil
		case "!=":
			return left != right, nil
		case "-eq", "-ne", "-lt", "-le", "-gt", "-ge":
			li, lerr := strconv.Atoi(left)
			ri, rerr := strconv.Atoi(right)
			if lerr != nil || rerr != nil {
				return false, fmt.Errorf("integer expected")
			}
			switch op {
			case "-eq":
				return li == ri, nil
			case "-ne":
				return li != ri, nil
			case "-lt":
				return li < ri, nil
			case "-le":
				return li <= ri, nil
			case "-gt":
				return li > ri, nil
			case "-ge":
				return li >= ri, nil
			}
		}
	}
	return false, nil
}

func buildEnv(vars map[string]string) []string {
	env := os.Environ()
	if _, ok := lookupEnv(env, "CONFIG_FEATURE_FANCY_ECHO"); !ok {
		env = append(env, "CONFIG_FEATURE_FANCY_ECHO=y")
	}
	for key, val := range vars {
		if _, ok := lookupEnv(env, key); ok {
			continue
		}
		env = append(env, key+"="+val)
	}
	return env
}

func isName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return false
			}
		} else if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
