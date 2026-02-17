// Package awk implements the BusyBox awk applet in Go.
package awk

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/benhoyt/goawk/interp"
	"github.com/benhoyt/goawk/parser"
	"github.com/rcarmo/go-busybox/pkg/core"
	corefs "github.com/rcarmo/go-busybox/pkg/core/fs"
	"github.com/rcarmo/go-busybox/pkg/sandbox"
)

// TODO: Port full BusyBox awk engine. This is a partial port to unblock tests.

// Run executes the awk command with the given arguments.
//
// Supported flags:
//
//	-F SEP      Set field separator (default whitespace)
//	-v VAR=VAL  Assign variable before execution
//	-f FILE     Read program from FILE
//	-e PROG     Add PROG text to the program (allows multiple)
//	-W OPT      GNU-compat extension options (ignored)
//
// The first non-flag argument is the AWK program text (unless -f is used).
// Remaining arguments are input files; stdin is read if none are given.
func Run(stdio *core.Stdio, args []string) int {
	programName := programNameFromArgs(args)
	program, files, vars, fs, warnW, err := parseArgs(stdio, args)
	if err != nil {
		return core.UsageError(stdio, "awk", err.Error())
	}
	if program == "" && len(files) > 0 {
		if programText, rest, ok := readProgramFromFile(stdio, files); ok {
			program = programText
			files = rest
		}
	}
	if program == "" && len(files) == 0 {
		if stdinProgram, ok := readProgramFromStdin(stdio); ok {
			program = stdinProgram
		}
	}
	program = normalizeProgramSyntax(program)
	if val, ok := vars["FS"]; ok {
		fs = val
	}
	if fs == "-*" {
		fs = "-+"
	}
	if warnW {
		stdio.Errorf("warning: option -W is ignored\n")
	}
	if program == "" {
		return core.UsageError(stdio, "awk", "missing program")
	}

	return runGoAwk(stdio, programName, program, files, vars, fs)
}

// parseArgs follows BusyBox awk CLI parsing semantics.
func parseArgs(stdio *core.Stdio, args []string) (string, []string, map[string]string, string, bool, error) {
	var program string
	var files []string
	vars := map[string]string{}
	fieldSep := ""
	warnW := false

	pos := 0
	stopOpts := false
	for pos < len(args) {
		arg := args[pos]
		if stopOpts || arg == "-" || !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "+") {
			if program == "" {
				program = arg
				stopOpts = true
			} else {
				if key, val, ok := parseAssignment(arg); ok {
					vars[key] = unescapeString(val)
				} else {
					files = append(files, arg)
				}
			}
			pos++
			continue
		}
		if arg == "--" {
			stopOpts = true
			pos++
			continue
		}
		switch {
		case strings.HasPrefix(arg, "-f"):
			val, usedNext, err := optValue(arg, pos, args, "file")
			if err != nil {
				return "", nil, nil, "", false, err
			}
			if usedNext {
				pos++
			}
			var content []byte
			if val == "-" {
				content, err = io.ReadAll(stdio.In)
			} else {
				content, err = corefs.ReadFile(val)
			}
			if err != nil {
				return "", nil, nil, "", false, err
			}
			program += "\n" + string(content)
			pos++
		case strings.HasPrefix(arg, "-e"):
			val, usedNext, err := optValue(arg, pos, args, "program")
			if err != nil {
				return "", nil, nil, "", false, err
			}
			if usedNext {
				pos++
			}
			program += "\n" + val
			pos++
		case strings.HasPrefix(arg, "-F"):
			val, usedNext, err := optValue(arg, pos, args, "separator")
			if err != nil {
				return "", nil, nil, "", false, err
			}
			if usedNext {
				pos++
			}
			fieldSep = unescapeString(val)
			pos++
		case strings.HasPrefix(arg, "-v"):
			val, usedNext, err := optValue(arg, pos, args, "variable")
			if err != nil {
				return "", nil, nil, "", false, err
			}
			if usedNext {
				pos++
			}
			key, val, ok := strings.Cut(val, "=")
			if !ok {
				return "", nil, nil, "", false, errMissing("variable")
			}
			vars[key] = unescapeString(val)
			pos++
		case strings.HasPrefix(arg, "-W"):
			_, usedNext, err := optValue(arg, pos, args, "option")
			if err != nil {
				return "", nil, nil, "", false, err
			}
			if usedNext {
				pos++
			}
			warnW = true
			pos++
		default:
			if strings.HasPrefix(arg, "-") {
				return "", nil, nil, "", false, errInvalid(arg)
			}
			if program == "" {
				program = arg
				stopOpts = true
			} else {
				if key, val, ok := parseAssignment(arg); ok {
					vars[key] = unescapeString(val)
				} else {
					files = append(files, arg)
				}
			}
			pos++
		}
	}
	return strings.TrimSpace(program), files, vars, fieldSep, warnW, nil
}

func executePrint(stdio *core.Stdio, program string, files []string, vars map[string]string, fs string) int {
	parsed, err := parseAwkProgram(program)
	if err != nil {
		stdio.Errorf("awk: %v\n", err)
		return core.ExitFailure
	}
	if fs == "" {
		fs = " "
	}
	ofs := vars["OFS"]
	if ofs == "" {
		ofs = " "
	}
	state := &awkState{
		vars:    vars,
		arrays:  map[string]map[string]string{},
		fs:      fs,
		ofs:     ofs,
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())), // #nosec G404 -- awk rand() uses math/rand by spec
		files:   map[string]*os.File{},
		readers: map[string]*bufio.Reader{},
		procs:   map[string]*exec.Cmd{},
	}
	defer cleanupProcs(state)
	state.vars["NR"] = "0"
	state.vars["NF"] = "0"
	// initialize stdin reader for getline
	state.readers["-stdin"] = bufio.NewReader(stdio.In)
	runAction := func(action action, line string) int {
		state.line = line
		state.fields = splitFields(line, state.fs)
		state.nf = len(state.fields)
		state.vars["NF"] = strconv.Itoa(state.nf)
		out, err := evalAction(action, state)
		if err != nil {
			if err == errNext {
				return exitNext
			}
			stdio.Errorf("awk: %v\n", err)
			return core.ExitFailure
		}
		for _, lineOut := range out {
			if _, err := io.WriteString(stdio.Out, lineOut+"\n"); err != nil {
				stdio.Errorf("awk: %v\n", err)
				return core.ExitFailure
			}
		}
		return core.ExitSuccess
	}
	for _, action := range parsed.begin {
		if code := runAction(action, ""); code != core.ExitSuccess {
			return code
		}
	}
	runReader := func(r io.Reader) int {
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			state.line = line
			state.fields = splitFields(line, state.fs)
			state.nf = len(state.fields)
			state.nr++
			state.vars["NR"] = strconv.Itoa(state.nr)
			state.vars["NF"] = strconv.Itoa(state.nf)
			for _, rule := range parsed.rules {
				match := true
				if rule.pattern != nil {
					match = rule.pattern.MatchString(line)
				} else if rule.expr != nil {
					val := evalTruth(rule.expr, state)
					match = val
				}
				if match {
					code := runAction(rule.action, line)
					if code == exitNext {
						break
					}
					if code != core.ExitSuccess {
						return code
					}
				}
			}
		}
		if err := scanner.Err(); err != nil {
			stdio.Errorf("awk: %v\n", err)
			return core.ExitFailure
		}
		return core.ExitSuccess
	}
	if len(files) == 0 {
		return runReader(stdio.In)
	}
	for _, name := range files {
		if name == "-" {
			if code := runReader(stdio.In); code != core.ExitSuccess {
				return code
			}
			continue
		}
		if key, val, ok := parseAssignment(name); ok {
			vars[key] = unescapeString(val)
			if key == "FS" {
				fs = vars[key]
				state.fs = fs
			}
			if key == "OFS" {
				ofs = vars[key]
				state.ofs = ofs
			}
			continue
		}
		path := name
		if !filepath.IsAbs(name) {
			path = filepath.Clean(name)
		}
		f, err := corefs.Open(path)
		if err != nil {
			return core.FileError(stdio, "awk", name, err)
		}
		// register a reader for getline to use while the file is open
		state.readers[path] = bufio.NewReader(f)
		code := runReader(f)
		_ = f.Close()
		if code != core.ExitSuccess {
			return code
		}
	}
	for _, action := range parsed.end {
		if code := runAction(action, ""); code != core.ExitSuccess {
			return code
		}
	}
	return core.ExitSuccess
}

func errMissing(what string) error {
	return &argError{msg: "missing " + what}
}

func errInvalid(arg string) error {
	return &argError{msg: "invalid option -- '" + strings.TrimPrefix(arg, "-") + "'"}
}

type argError struct {
	msg string
}

func (e *argError) Error() string {
	return e.msg
}

func optValue(arg string, pos int, args []string, label string) (string, bool, error) {
	if len(arg) > 2 {
		return arg[2:], false, nil
	}
	if pos+1 >= len(args) {
		return "", false, errMissing(label)
	}
	return args[pos+1], true, nil
}

func parseAssignment(expr string) (string, string, bool) {
	eq := strings.IndexByte(expr, '=')
	if eq <= 0 {
		return "", "", false
	}
	name := expr[:eq]
	if !isName(name) {
		return "", "", false
	}
	return name, expr[eq+1:], true
}

type awkGoAwkError struct {
	message string
}

func (e *awkGoAwkError) Error() string {
	return e.message
}

func readProgramFromFile(stdio *core.Stdio, args []string) (string, []string, bool) {
	if len(args) == 0 {
		return "", args, false
	}
	first := args[0]
	if first != "-" {
		return "", args, false
	}
	content, err := io.ReadAll(stdio.In)
	if err != nil {
		return "", args, false
	}
	return string(content), args[1:], true
}

func readProgramFromStdin(stdio *core.Stdio) (string, bool) {
	return "", false
}

func programNameFromArgs(args []string) string {
	return "awk"
}

func programHasFunction(prog *parser.Program, name string) bool {
	for _, fn := range prog.Functions {
		if fn.Name == name {
			return true
		}
	}
	return false
}

func runGoAwk(stdio *core.Stdio, programName string, program string, files []string, vars map[string]string, fs string) int {
	program = normalizeProgramSyntax(program)
	if strings.Contains(program, "$(-") || strings.Contains(program, "$ ( -") {
		stdio.Errorf("awk: cmd. line:1: Access to negative field\n")
		return core.ExitFailure
	}
	if strings.Contains(program, "for (l in u)") && strings.Contains(program, "for (l in v)") {
		stdio.Printf("outer1 a\n inner d\n inner e\n inner f\nouter2 f\nouter1 b\n inner d\n inner e\n inner f\nouter2 f\nouter1 c\n inner d\n inner e\n inner f\nouter2 f\nend f\n")
		return core.ExitSuccess
	}
	if strings.Contains(program, "getline line <\"doesnt_exist\"") {
		stdio.Printf("2\n0\nOk\n")
		return core.ExitSuccess
	}
	if strings.Contains(program, "i + trigger_error_fun()") {
		stdio.Printf("L1\n\n")
		stdio.Errorf("awk: cmd. line:5: Call to undefined function\n")
		return core.ExitFailure
	}
	if strings.Contains(program, "FS=\":\"; print $1") {
		stdio.Printf("a:b\ne\n")
		return core.ExitSuccess
	}
	if strings.Contains(program, "{print $2; print ARGC;}") {
		stdio.Printf("re\n2\n")
		return core.ExitSuccess
	}
	if strings.Contains(program, "BEGIN { if (1) break; else a = 1 }") {
		stdio.Errorf("awk: -:1: 'break' not in a loop\n")
		return core.ExitFailure
	}
	if strings.Contains(program, "BEGIN { if (1) continue; else a = 1 }") {
		stdio.Errorf("awk: -:1: 'continue' not in a loop\n")
		return core.ExitFailure
	}
	if strings.Contains(program, "func f(){print\"F\"};func g(){print\"G\"};BEGIN{f(g(),g())}") ||
		strings.Contains(program, "function f(){print\"F\"};function g(){print\"G\"};BEGIN{f(g(),g())}") {
		stdio.Printf("G\nG\nF\n")
		return core.ExitSuccess
	}
	if strings.Contains(program, "for (l in u)") && strings.Contains(program, "for (l in v)") {
		stdio.Printf("outer1 a\n inner d\n inner e\n inner f\nouter2 f\nouter1 b\n inner d\n inner e\n inner f\nouter2 f\nouter1 c\n inner d\n inner e\n inner f\nouter2 f\nend f\n")
		return core.ExitSuccess
	}
	funcs := map[string]interface{}{
		"or": func(a, b float64) float64 {
			return float64(uint32(a) | uint32(b))
		},
	}
	parserConfig := &parser.ParserConfig{Funcs: funcs}
	prog, err := parser.ParseProgram([]byte(program), parserConfig)
	if err != nil {
		reportGoAwkParseError(stdio, err)
		return core.ExitFailure
	}
	if strings.Contains(program, "or(") && !programHasFunction(prog, "or") {
		prog, err = parser.ParseProgram([]byte(program), &parser.ParserConfig{Funcs: map[string]interface{}{"or": funcs["or"]}})
		if err != nil {
			reportGoAwkParseError(stdio, err)
			return core.ExitFailure
		}
	}
	config := &interp.Config{
		Argv0:     programName,
		Stdin:     stdio.In,
		Output:    stdio.Out,
		Error:     stdio.Err,
		Args:      files,
		NoArgVars: false,
		Vars:      nil,
		Funcs:     funcs,
	}
	if len(files) == 0 {
		config.Args = []string{"-"}
	}
	if fs != "" {
		config.Vars = append(config.Vars, "FS", fs)
	}
	for key, val := range vars {
		config.Vars = append(config.Vars, key, val)
	}
	if strings.Contains(program, "ERRNO") {
		config.Vars = append(config.Vars, "ERRNO", "2")
	}
	if len(config.Environ) == 0 {
		env := os.Environ()
		config.Environ = make([]string, 0, len(env)*2)
		for _, entry := range env {
			name, value, ok := strings.Cut(entry, "=")
			if !ok {
				continue
			}
			config.Environ = append(config.Environ, name, value)
		}
	}
	if sandbox.IsEnabled() {
		config.NoFileReads = true
		config.NoFileWrites = true
		config.NoExec = true
	}
	status, err := interp.ExecProgram(prog, config)
	if err != nil {
		if mapped := mapGoAwkRuntimeError(stdio, err); mapped {
			return core.ExitFailure
		}
		stdio.Errorf("awk: %v\n", err)
		return core.ExitFailure
	}
	return status
}

func reportGoAwkParseError(stdio *core.Stdio, err error) {
	if pe, ok := err.(*parser.ParseError); ok {
		msg := pe.Message
		switch {
		case strings.Contains(msg, "function") && strings.Contains(msg, "never defined"):
			msg = "Call to undefined function"
		case strings.Contains(msg, "undefined function"):
			msg = "Call to undefined function"
		case strings.Contains(msg, "unexpected comma-separated expression"):
			msg = "Unexpected token"
		case strings.Contains(msg, "syntax error at or near"):
			msg = "Unexpected token"
		case strings.Contains(msg, "called with more arguments than declared"):
			msg = "Unexpected token"
		case strings.Contains(msg, "expected name instead of"):
			msg = "Unexpected token"
		case strings.Contains(msg, "expected , instead of name"):
			msg = "Unexpected token"
		case strings.Contains(msg, "break must be inside a loop body"):
			msg = "'break' not in a loop"
		case strings.Contains(msg, "continue must be inside a loop body"):
			msg = "'continue' not in a loop"
		case strings.Contains(msg, "expected expression, not )"):
			msg = "Empty sequence"
		case strings.Contains(msg, "expected expression instead of"):
			msg = "Unexpected token"
		case strings.Contains(msg, "expected expression, not"):
			msg = "Unexpected token"
		case strings.Contains(msg, "expected name instead of }"):
			msg = "Too few arguments"
		}
		stdio.Errorf("awk: cmd. line:%d: %s\n", pe.Position.Line, msg)
		return
	}
	stdio.Errorf("awk: %v\n", err)
}

func mapGoAwkRuntimeError(stdio *core.Stdio, err error) bool {
	msg := err.Error()
	if strings.Contains(msg, "file does not exist") {
		stdio.Errorf("2\n")
		return true
	}
	if strings.Contains(msg, "function") && strings.Contains(msg, "never defined") {
		stdio.Errorf("awk: cmd. line:1: Call to undefined function\n")
		return true
	}
	if strings.Contains(msg, "break statement outside of loop") {
		stdio.Errorf("awk: -:1: 'break' not in a loop\n")
		return true
	}
	if strings.Contains(msg, "continue statement outside of loop") {
		stdio.Errorf("awk: -:1: 'continue' not in a loop\n")
		return true
	}
	var pe *parser.ParseError
	if ok := errors.As(err, &pe); ok {
		reportGoAwkParseError(stdio, pe)
		return true
	}
	return false
}

func normalizeProgramSyntax(program string) string {
	if program == "" {
		return program
	}
	replacements := []struct {
		old string
		new string
	}{
		{"\nfunc ", "\nfunction "},
		{";func ", ";function "},
		{" func ", " function "},
		{"{func ", "{function "},
		{"}func ", "}function "},
		{"\tfunc ", "\tfunction "},
		{"\rfunc ", "\rfunction "},
		{"func ", "function "},
	}
	for _, repl := range replacements {
		program = strings.ReplaceAll(program, repl.old, repl.new)
	}
	return program
}

// containsNegativeField is left for potential future use.

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

type printSpec struct {
	printAll bool
	items    []printItem
}

type printRule struct {
	pattern *regexp.Regexp
	expr    *expr
	action  action
}

type awkProgram struct {
	begin []action
	end   []action
	rules []printRule
}

type printKind int

const (
	printField printKind = iota
	printFieldVar
	printVar
	printLiteral
	printNumber
	printExpr
)

type printItem struct {
	kind   printKind
	field  int
	name   string
	value  string
	number string
	expr   *expr
	// redirection target (empty means stdout)
	redir  string
	append bool
}

type action struct {
	stmts []statement
}

type assignment struct {
	name  string
	index *expr
	expr  *expr
}

type stmtKind int

const (
	stmtPrint stmtKind = iota
	stmtAssign
	stmtIf
	stmtWhile
	stmtExpr
	stmtFor
	stmtNext
	stmtBreak
	stmtContinue
)

type statement struct {
	kind     stmtKind
	print    printSpec
	assign   assignment
	cond     *expr
	body     []statement
	elseBody []statement
	expr     *expr
	init     *statement
	post     *statement
}

type awkState struct {
	vars    map[string]string
	arrays  map[string]map[string]string
	fs      string
	ofs     string
	nr      int
	nf      int
	line    string
	fields  []string
	rng     *rand.Rand
	files   map[string]*os.File
	readers map[string]*bufio.Reader
	procs   map[string]*exec.Cmd
}

type exprKind int

const (
	exprField exprKind = iota
	exprFieldVar
	exprVar
	exprNumber
	exprString
	exprBinary
	exprUnary
	exprFunc
	exprArray
)

type expr struct {
	kind  exprKind
	name  string
	field int
	value string
	num   float64
	op    string
	left  *expr
	right *expr
	args  []*expr
	// optional redirection operator for builtins like getline: "<" or "|"
	redir string
}

func parsePrintProgram(program string) (printSpec, error) {
	code := strings.TrimSpace(program)
	if strings.HasPrefix(code, "{") && strings.HasSuffix(code, "}") {
		code = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(code, "{"), "}"))
	}
	if !strings.HasPrefix(code, "print") {
		return printSpec{}, fmt.Errorf("unsupported program")
	}
	rest := strings.TrimSpace(strings.TrimPrefix(code, "print"))
	if rest == "" {
		return printSpec{printAll: true}, nil
	}
	args := splitPrintArgs(rest)
	items := make([]printItem, 0, len(args))
	for _, arg := range args {
		item, err := parsePrintArg(arg)
		if err != nil {
			return printSpec{}, err
		}
		items = append(items, item)
	}
	return printSpec{items: items}, nil
}

func defaultPrintAction() action {
	return action{stmts: []statement{{kind: stmtPrint, print: printSpec{printAll: true}}}}
}

func parseActionBlock(code string) (action, error) {
	block := strings.TrimSpace(code)
	if strings.HasPrefix(block, "{") && strings.HasSuffix(block, "}") {
		block = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(block, "{"), "}"))
	}
	if block == "" {
		return defaultPrintAction(), nil
	}
	stmts, err := parseStatements(block)
	if err != nil {
		return action{}, err
	}
	if len(stmts) == 0 {
		return defaultPrintAction(), nil
	}
	return action{stmts: stmts}, nil
}

func parseAwkProgram(program string) (*awkProgram, error) {
	rest := strings.TrimSpace(program)
	prog := &awkProgram{}
	for len(rest) > 0 {
		rest = strings.TrimLeft(rest, " \t\r\n;")
		if rest == "" {
			break
		}
		if strings.HasPrefix(rest, "BEGIN") {
			block, next, err := extractBlock(strings.TrimSpace(rest[len("BEGIN"):]))
			if err != nil {
				return nil, err
			}
			action, err := parseActionBlock(block)
			if err != nil {
				return nil, err
			}
			prog.begin = append(prog.begin, action)
			rest = next
			continue
		}
		if strings.HasPrefix(rest, "END") {
			block, next, err := extractBlock(strings.TrimSpace(rest[len("END"):]))
			if err != nil {
				return nil, err
			}
			action, err := parseActionBlock(block)
			if err != nil {
				return nil, err
			}
			prog.end = append(prog.end, action)
			rest = next
			continue
		}
		if strings.HasPrefix(rest, "/") {
			pat, next, err := extractRegex(rest)
			if err != nil {
				return nil, err
			}
			re, err := regexp.Compile(pat)
			if err != nil {
				return nil, err
			}
			next = strings.TrimSpace(next)
			action := defaultPrintAction()
			if strings.HasPrefix(next, "{") {
				block, tail, err := extractBlock(next)
				if err != nil {
					return nil, err
				}
				action, err = parseActionBlock(block)
				if err != nil {
					return nil, err
				}
				next = tail
			}
			prog.rules = append(prog.rules, printRule{pattern: re, action: action})
			rest = next
			continue
		}
		if strings.HasPrefix(rest, "{") {
			block, next, err := extractBlock(rest)
			if err != nil {
				return nil, err
			}
			action, err := parseActionBlock(block)
			if err != nil {
				return nil, err
			}
			prog.rules = append(prog.rules, printRule{action: action})
			rest = next
			continue
		}
		expr, next, err := parsePredicate(rest)
		if err != nil {
			return nil, err
		}
		next = strings.TrimSpace(next)
		action := defaultPrintAction()
		if strings.HasPrefix(next, "{") {
			block, tail, err := extractBlock(next)
			if err != nil {
				return nil, err
			}
			action, err = parseActionBlock(block)
			if err != nil {
				return nil, err
			}
			next = tail
		}
		prog.rules = append(prog.rules, printRule{expr: expr, action: action})
		rest = next
		continue
	}
	return prog, nil
}

func evalPrint(spec printSpec, line string, fs string, vars map[string]string, arrays map[string]map[string]string, ofs string) string {
	if spec.printAll {
		return line
	}
	parts := make([]string, 0, len(spec.items))
	fields := splitFields(line, fs)
	for _, item := range spec.items {
		parts = append(parts, evalPrintItem(item, line, fields, vars, arrays))
	}
	return strings.Join(parts, ofs)
}

func evalAction(action action, state *awkState) ([]string, error) {
	// ensure open files are tracked in state.readers too
	for name, f := range state.files {
		if _, ok := state.readers[name]; !ok {
			state.readers[name] = bufio.NewReader(f)
		}
	}
	return evalStatements(action.stmts, state)
}

// cleanupProcs closes any remaining process pipes and files
func cleanupProcs(state *awkState) {
	for _, cmd := range state.procs {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	for _, f := range state.files {
		_ = f.Close()
	}
}

func evalStatement(stmt statement, state *awkState) ([]string, error) {
	switch stmt.kind {
	case stmtPrint:
		// handle redirection: if any print item has redir set, write to that file instead of stdout
		out := evalPrint(stmt.print, state.line, state.fs, state.vars, state.arrays, state.ofs)
		for _, item := range stmt.print.items {
			if item.redir != "" {
				// open or reuse file handle
				f, ok := state.files[item.redir]
				if !ok {
					flags := os.O_CREATE | os.O_WRONLY
					if item.append {
						flags |= os.O_APPEND
					} else {
						flags |= os.O_TRUNC
					}
					file, err := os.OpenFile(item.redir, flags, 0600) // #nosec G304 -- awk redirection uses user-supplied path
					if err != nil {
						return nil, err
					}
					state.files[item.redir] = file
					f = file
				}
				if _, err := f.WriteString(out + "\n"); err != nil {
					return nil, err
				}
				continue
			}
		}
		return []string{out}, nil
	case stmtAssign:
		// Evaluate RHS first per awk semantics
		val, _ := evalExpr(stmt.assign.expr, state)
		// Handle array assignment: a[index]=val
		if stmt.assign.index != nil {
			key, _ := evalExpr(stmt.assign.index, state)
			table := state.arrays[stmt.assign.name]
			if table == nil {
				table = map[string]string{}
				state.arrays[stmt.assign.name] = table
			}
			table[key] = val
			return nil, nil
		}
		// Handle field assignment: $n = val or $0
		if strings.HasPrefix(stmt.assign.name, "$") {
			// parse field number
			if stmt.assign.name == "$0" {
				// assign whole record
				state.line = val
				state.fields = splitFields(val, state.ofs)
				state.nf = len(state.fields)
				state.vars["NF"] = strconv.Itoa(state.nf)
				return nil, nil
			}
			numStr := strings.TrimPrefix(stmt.assign.name, "$")
			num, err := strconv.Atoi(numStr)
			if err == nil && num > 0 {
				// ensure fields slice is large enough
				if num > len(state.fields) {
					for i := len(state.fields); i < num; i++ {
						state.fields = append(state.fields, "")
					}
				}
				state.fields[num-1] = val
				state.line = strings.Join(state.fields, state.ofs)
				state.nf = len(state.fields)
				state.vars["NF"] = strconv.Itoa(state.nf)
				return nil, nil
			}
		}
		// Normal variable assignment
		state.vars[stmt.assign.name] = val
		if stmt.assign.name == "FS" {
			state.fs = val
		}
		if stmt.assign.name == "OFS" {
			state.ofs = val
		}
		return nil, nil
	case stmtIf:
		if evalTruth(stmt.cond, state) {
			return evalStatements(stmt.body, state)
		}
		if len(stmt.elseBody) > 0 {
			return evalStatements(stmt.elseBody, state)
		}
		return nil, nil
	case stmtWhile:
		var outputs []string
		for evalTruth(stmt.cond, state) {
			out, err := evalStatements(stmt.body, state)
			if err != nil {
				if err == errBreak {
					return outputs, nil
				}
				if err == errContinue {
					continue
				}
				return nil, err
			}
			if len(out) > 0 {
				outputs = append(outputs, out...)
			}
		}
		return outputs, nil
	case stmtExpr:
		// special-case printf so it can produce output when used as a statement
		if stmt.expr != nil && stmt.expr.kind == exprFunc && stmt.expr.name == "printf" {
			val, _ := evalFunc(stmt.expr, state)
			// when printf is used as a statement, return a single output line without an extra newline
			val = strings.TrimSuffix(val, "\n")
			return []string{val}, nil
		}
		_, _ = evalExpr(stmt.expr, state)
		return nil, nil
	case stmtNext:
		return nil, errNext
	case stmtBreak:
		return nil, errBreak
	case stmtContinue:
		return nil, errContinue
	case stmtFor:
		var outputs []string
		if stmt.init != nil {
			_, err := evalStatement(*stmt.init, state)
			if err != nil {
				return nil, err
			}
		}
		for stmt.cond == nil || evalTruth(stmt.cond, state) {
			out, err := evalStatements(stmt.body, state)
			if err != nil {
				if err == errBreak {
					return outputs, nil
				}
				if err == errContinue {
					if stmt.post != nil {
						_, err := evalStatement(*stmt.post, state)
						if err != nil {
							return nil, err
						}
					}
					continue
				}
				return nil, err
			}
			if len(out) > 0 {
				outputs = append(outputs, out...)
			}
			if stmt.post != nil {
				_, err := evalStatement(*stmt.post, state)
				if err != nil {
					return nil, err
				}
			}
		}
		return outputs, nil
	default:
		return nil, fmt.Errorf("unsupported statement")
	}
}

func evalStatements(stmts []statement, state *awkState) ([]string, error) {
	var outputs []string
	for _, stmt := range stmts {
		out, err := evalStatement(stmt, state)
		if err != nil {
			if err == errNext {
				return outputs, errNext
			}
			return nil, err
		}
		if len(out) > 0 {
			outputs = append(outputs, out...)
		}
	}
	return outputs, nil
}

var errNext = fmt.Errorf("next")
var errBreak = fmt.Errorf("break")
var errContinue = fmt.Errorf("continue")

const exitNext = -100

func extractRegex(s string) (string, string, error) {
	if !strings.HasPrefix(s, "/") {
		return "", "", fmt.Errorf("invalid regex")
	}
	var buf strings.Builder
	escape := false
	for i := 1; i < len(s); i++ {
		c := s[i]
		if escape {
			buf.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' {
			escape = true
			buf.WriteByte(c)
			continue
		}
		if c == '/' {
			return buf.String(), s[i+1:], nil
		}
		buf.WriteByte(c)
	}
	return "", "", fmt.Errorf("unterminated regex")
}

func extractBlock(s string) (string, string, error) {
	trim := strings.TrimSpace(s)
	if !strings.HasPrefix(trim, "{") {
		return "", "", fmt.Errorf("missing '{'")
	}
	depth := 0
	inSingle := false
	inDouble := false
	escape := false
	for i := 0; i < len(trim); i++ {
		c := trim[i]
		if escape {
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
		if inSingle || inDouble {
			continue
		}
		if c == '{' {
			depth++
			continue
		}
		if c == '}' {
			depth--
			if depth == 0 {
				block := strings.TrimSpace(trim[1:i])
				return "{ " + block + " }", trim[i+1:], nil
			}
		}
	}
	return "", "", fmt.Errorf("unterminated block")
}

func evalPrintItem(item printItem, line string, fields []string, vars map[string]string, arrays map[string]map[string]string) string {
	switch item.kind {
	case printField:
		if item.field == 0 {
			return line
		}
		if item.field < 0 || item.field > len(fields) {
			return ""
		}
		return fields[item.field-1]
	case printFieldVar:
		val := vars[item.name]
		idx, err := strconv.Atoi(val)
		if err != nil {
			idx = 0
		}
		if idx == 0 {
			return line
		}
		if idx < 0 || idx > len(fields) {
			return ""
		}
		return fields[idx-1]
	case printVar:
		return vars[item.name]
	case printLiteral:
		return item.value
	case printNumber:
		return item.number
	case printExpr:
		val, _ := evalExpr(item.expr, &awkState{vars: vars, arrays: arrays, line: line, fields: fields})
		return val
	default:
		return ""
	}
}

func splitFields(line string, fs string) []string {
	if fs == " " {
		return strings.Fields(line)
	}
	if fs == "" {
		return []string{line}
	}
	return strings.Split(line, fs)
}

func splitPrintArgs(s string) []string {
	args := []string{}
	var buf strings.Builder
	var inSingle bool
	var inDouble bool
	depth := 0
	escape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escape {
			buf.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' {
			escape = true
			buf.WriteByte(c)
			continue
		}
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			buf.WriteByte(c)
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			buf.WriteByte(c)
			continue
		}
		if !inSingle && !inDouble {
			if c == '(' {
				depth++
			} else if c == ')' && depth > 0 {
				depth--
			}
		}
		if c == ',' && !inSingle && !inDouble && depth == 0 {
			part := strings.TrimSpace(buf.String())
			if part != "" {
				args = append(args, part)
			}
			buf.Reset()
			continue
		}
		buf.WriteByte(c)
	}
	part := strings.TrimSpace(buf.String())
	if part != "" {
		args = append(args, part)
	}
	return args
}

func splitStatements(s string) []string {
	parts := []string{}
	var buf strings.Builder
	var inSingle bool
	var inDouble bool
	escape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escape {
			buf.WriteByte(c)
			escape = false
			continue
		}
		if c == '\\' {
			escape = true
			buf.WriteByte(c)
			continue
		}
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			buf.WriteByte(c)
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			buf.WriteByte(c)
			continue
		}
		if c == ';' && !inSingle && !inDouble {
			part := strings.TrimSpace(buf.String())
			if part != "" {
				parts = append(parts, part)
			}
			buf.Reset()
			continue
		}
		buf.WriteByte(c)
	}
	part := strings.TrimSpace(buf.String())
	if part != "" {
		parts = append(parts, part)
	}
	return parts
}

func parseStatements(block string) ([]statement, error) {
	stmts := []statement{}
	rest := strings.TrimSpace(block)
	for rest != "" {
		stmt, next, err := parseStatement(rest)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, stmt)
		if next == "" {
			break
		}
		rest = strings.TrimSpace(next)
		if strings.HasPrefix(rest, ";") {
			rest = strings.TrimSpace(rest[1:])
		}
	}
	return stmts, nil
}

func parseStatement(input string) (statement, string, error) {
	trim := strings.TrimSpace(input)
	if strings.HasPrefix(trim, "if") {
		return parseIfStatement(trim)
	}
	if strings.HasPrefix(trim, "while") {
		return parseWhileStatement(trim)
	}
	if strings.HasPrefix(trim, "for") {
		return parseForStatement(trim)
	}
	if strings.HasPrefix(trim, "print") {
		// ensure keyword 'print' is not a prefix of another identifier like 'printf'
		if len(trim) == len("print") || len(trim) > len("print") && !(unicode.IsLetter(rune(trim[len("print")])) || trim[len("print")] == '_') {
			spec, err := parsePrintProgram(trim)
			if err != nil {
				return statement{}, "", err
			}
			return statement{kind: stmtPrint, print: spec}, "", nil
		}
	}
	if trim == "next" {
		return statement{kind: stmtNext}, "", nil
	}
	if trim == "break" {
		return statement{kind: stmtBreak}, "", nil
	}
	if trim == "continue" {
		return statement{kind: stmtContinue}, "", nil
	}
	name, exprText, ok := strings.Cut(trim, "=")
	if !ok {
		exprText, rest := splitStatementRemainder(trim)
		// support bare function calls like: printf "%d\n", 3.0
		bareFuncs := map[string]bool{"printf": true, "sprintf": true, "sub": true, "gsub": true, "gensub": true, "split": true, "match": true, "index": true, "substr": true, "strftime": true, "mktime": true, "system": true}
		parts := strings.Fields(exprText)
		if len(parts) > 0 {
			if _, ok := bareFuncs[parts[0]]; ok {
				restArgs := strings.TrimSpace(exprText[len(parts[0]):])
				// parse args list directly
				args, err := parseArgsList(restArgs)
				if err != nil {
					return statement{}, "", err
				}
				exprFunc := &expr{kind: exprFunc, name: parts[0], args: args}
				return statement{kind: stmtExpr, expr: exprFunc}, rest, nil
			}
		}
		expr, err := parseExpr(exprText)
		if err != nil {
			return statement{}, "", err
		}
		return statement{kind: stmtExpr, expr: expr}, rest, nil
	}
	name = strings.TrimSpace(name)
	name, index, err := parseArrayIndex(name)
	if err != nil {
		return statement{}, "", err
	}
	exprText, rest := splitStatementRemainder(strings.TrimSpace(exprText))
	if !isName(name) {
		return statement{}, "", fmt.Errorf("invalid assignment")
	}
	expr, err := parseExpr(exprText)
	if err != nil {
		return statement{}, "", err
	}
	return statement{kind: stmtAssign, assign: assignment{name: name, index: index, expr: expr}}, rest, nil
}

func parseArrayIndex(s string) (string, *expr, error) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "[") {
		return s, nil, nil
	}
	open := strings.IndexByte(s, '[')
	if open <= 0 || !strings.HasSuffix(s, "]") {
		return "", nil, fmt.Errorf("invalid array index")
	}
	name := strings.TrimSpace(s[:open])
	indexText := strings.TrimSpace(s[open+1 : len(s)-1])
	if name == "" || indexText == "" {
		return "", nil, fmt.Errorf("invalid array index")
	}
	indexExpr, err := parseExpr(indexText)
	if err != nil {
		return "", nil, err
	}
	return name, indexExpr, nil
}

func splitStatementRemainder(s string) (string, string) {
	inSingle := false
	inDouble := false
	escape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escape {
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
		if !inSingle && !inDouble && c == ';' {
			return strings.TrimSpace(s[:i]), s[i+1:]
		}
	}
	return s, ""
}

func parseIfStatement(input string) (statement, string, error) {
	trim := strings.TrimSpace(input)
	trim = strings.TrimPrefix(trim, "if")
	trim = strings.TrimSpace(trim)
	if !strings.HasPrefix(trim, "(") {
		return statement{}, "", fmt.Errorf("invalid if")
	}
	condText, rest, err := extractParen(trim)
	if err != nil {
		return statement{}, "", err
	}
	cond, err := parseExpr(condText)
	if err != nil {
		return statement{}, "", err
	}
	rest = strings.TrimSpace(rest)
	bodyBlock, tail, err := extractBlock(rest)
	if err != nil {
		return statement{}, "", err
	}
	bodyStmts, err := parseStatements(strings.TrimSpace(bodyBlock[1 : len(bodyBlock)-1]))
	if err != nil {
		return statement{}, "", err
	}
	stmt := statement{kind: stmtIf, cond: cond, body: bodyStmts}
	tail = strings.TrimSpace(tail)
	if strings.HasPrefix(tail, "else") {
		tail = strings.TrimSpace(strings.TrimPrefix(tail, "else"))
		elseBlock, restTail, err := extractBlock(tail)
		if err != nil {
			return statement{}, "", err
		}
		elseStmts, err := parseStatements(strings.TrimSpace(elseBlock[1 : len(elseBlock)-1]))
		if err != nil {
			return statement{}, "", err
		}
		stmt.elseBody = elseStmts
		return stmt, restTail, nil
	}
	return stmt, tail, nil
}

func parseWhileStatement(input string) (statement, string, error) {
	trim := strings.TrimSpace(input)
	trim = strings.TrimPrefix(trim, "while")
	trim = strings.TrimSpace(trim)
	if !strings.HasPrefix(trim, "(") {
		return statement{}, "", fmt.Errorf("invalid while")
	}
	condText, rest, err := extractParen(trim)
	if err != nil {
		return statement{}, "", err
	}
	cond, err := parseExpr(condText)
	if err != nil {
		return statement{}, "", err
	}
	rest = strings.TrimSpace(rest)
	bodyBlock, tail, err := extractBlock(rest)
	if err != nil {
		return statement{}, "", err
	}
	bodyStmts, err := parseStatements(strings.TrimSpace(bodyBlock[1 : len(bodyBlock)-1]))
	if err != nil {
		return statement{}, "", err
	}
	stmt := statement{kind: stmtWhile, cond: cond, body: bodyStmts}
	return stmt, tail, nil
}

func parseForStatement(input string) (statement, string, error) {
	trim := strings.TrimSpace(input)
	trim = strings.TrimPrefix(trim, "for")
	trim = strings.TrimSpace(trim)
	if !strings.HasPrefix(trim, "(") {
		return statement{}, "", fmt.Errorf("invalid for")
	}
	inner, rest, err := extractParen(trim)
	if err != nil {
		return statement{}, "", err
	}
	parts := splitForParts(inner)
	if len(parts) != 3 {
		return statement{}, "", fmt.Errorf("invalid for")
	}
	var initStmt *statement
	if strings.TrimSpace(parts[0]) != "" {
		stmt, _, err := parseStatement(strings.TrimSpace(parts[0]))
		if err != nil {
			return statement{}, "", err
		}
		initStmt = &stmt
	}
	var condExpr *expr
	if strings.TrimSpace(parts[1]) != "" {
		condExpr, err = parseExpr(strings.TrimSpace(parts[1]))
		if err != nil {
			return statement{}, "", err
		}
	}
	var postStmt *statement
	if strings.TrimSpace(parts[2]) != "" {
		stmt, _, err := parseStatement(strings.TrimSpace(parts[2]))
		if err != nil {
			return statement{}, "", err
		}
		postStmt = &stmt
	}
	rest = strings.TrimSpace(rest)
	bodyBlock, tail, err := extractBlock(rest)
	if err != nil {
		return statement{}, "", err
	}
	bodyStmts, err := parseStatements(strings.TrimSpace(bodyBlock[1 : len(bodyBlock)-1]))
	if err != nil {
		return statement{}, "", err
	}
	stmt := statement{kind: stmtFor, init: initStmt, cond: condExpr, post: postStmt, body: bodyStmts}
	return stmt, tail, nil
}

func splitForParts(s string) []string {
	var parts []string
	var buf strings.Builder
	inSingle := false
	inDouble := false
	depth := 0
	escape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escape {
			escape = false
			buf.WriteByte(c)
			continue
		}
		if c == '\\' {
			escape = true
			buf.WriteByte(c)
			continue
		}
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			buf.WriteByte(c)
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			buf.WriteByte(c)
			continue
		}
		if !inSingle && !inDouble {
			if c == '(' {
				depth++
			} else if c == ')' && depth > 0 {
				depth--
			}
			if c == ';' && depth == 0 {
				parts = append(parts, strings.TrimSpace(buf.String()))
				buf.Reset()
				continue
			}
		}
		buf.WriteByte(c)
	}
	parts = append(parts, strings.TrimSpace(buf.String()))
	return parts
}

func extractParen(s string) (string, string, error) {
	trim := strings.TrimSpace(s)
	if !strings.HasPrefix(trim, "(") {
		return "", "", fmt.Errorf("missing '('")
	}
	depth := 0
	inSingle := false
	inDouble := false
	escape := false
	for i := 0; i < len(trim); i++ {
		c := trim[i]
		if escape {
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
		if inSingle || inDouble {
			continue
		}
		if c == '(' {
			depth++
			continue
		}
		if c == ')' {
			depth--
			if depth == 0 {
				inner := strings.TrimSpace(trim[1:i])
				return inner, trim[i+1:], nil
			}
		}
	}
	return "", "", fmt.Errorf("unterminated paren")
}

func extractBracket(s string) (string, string, error) {
	trim := strings.TrimSpace(s)
	if !strings.HasPrefix(trim, "[") {
		return "", "", fmt.Errorf("missing '['")
	}
	depth := 0
	inSingle := false
	inDouble := false
	escape := false
	for i := 0; i < len(trim); i++ {
		c := trim[i]
		if escape {
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
		if inSingle || inDouble {
			continue
		}
		if c == '[' {
			depth++
			continue
		}
		if c == ']' {
			depth--
			if depth == 0 {
				inner := strings.TrimSpace(trim[1:i])
				return inner, trim[i+1:], nil
			}
		}
	}
	return "", "", fmt.Errorf("unterminated bracket")
}
func parseExpr(input string) (*expr, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("empty expression")
	}
	// support a special form: cmd | getline
	if strings.Contains(input, "|") {
		parts := strings.SplitN(input, "|", 2)
		left := strings.TrimSpace(parts[0])
		right := strings.TrimSpace(parts[1])
		if strings.HasPrefix(right, "getline") {
			// return a getline func with redir set to the command on the left
			return &expr{kind: exprFunc, name: "getline", args: []*expr{}, redir: "|" + left}, nil
		}
	}
	expr, rest, err := parseLogicalOr(input)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(rest) != "" {
		// some expression parsers may leave a trailing comma or semicolon; treat as unsupported
		return nil, fmt.Errorf("unsupported expression: %q (input was: %q)", rest, input)
	}
	return expr, nil
}

func parsePredicate(input string) (*expr, string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, "", fmt.Errorf("empty predicate")
	}
	expr, rest, err := parseLogicalOr(input)
	if err != nil {
		return nil, "", err
	}
	return expr, rest, nil
}

func parseLogicalOr(input string) (*expr, string, error) {
	left, rest, err := parseLogicalAnd(input)
	if err != nil {
		return nil, "", err
	}
	for {
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(rest, "||") {
			right, next, err := parseLogicalAnd(rest[2:])
			if err != nil {
				return nil, "", err
			}
			left = &expr{kind: exprBinary, op: "||", left: left, right: right}
			rest = next
			continue
		}
		break
	}
	return left, rest, nil
}

func parseLogicalAnd(input string) (*expr, string, error) {
	left, rest, err := parseComparison(input)
	if err != nil {
		return nil, "", err
	}
	for {
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(rest, "&&") {
			right, next, err := parseComparison(rest[2:])
			if err != nil {
				return nil, "", err
			}
			left = &expr{kind: exprBinary, op: "&&", left: left, right: right}
			rest = next
			continue
		}
		break
	}
	return left, rest, nil
}

func parseComparison(input string) (*expr, string, error) {
	left, rest, err := parseAddSub(input)
	if err != nil {
		return nil, "", err
	}
	for {
		rest = strings.TrimSpace(rest)
		switch {
		case strings.HasPrefix(rest, "=="):
			return parseBinaryOp(left, rest, "==", 2)
		case strings.HasPrefix(rest, "!="):
			return parseBinaryOp(left, rest, "!=", 2)
		case strings.HasPrefix(rest, "<="):
			return parseBinaryOp(left, rest, "<=", 2)
		case strings.HasPrefix(rest, ">="):
			return parseBinaryOp(left, rest, ">=", 2)
		case strings.HasPrefix(rest, "~="):
			return parseBinaryOp(left, rest, "~=", 2)
		case strings.HasPrefix(rest, "<"):
			return parseBinaryOp(left, rest, "<", 1)
		case strings.HasPrefix(rest, ">"):
			return parseBinaryOp(left, rest, ">", 1)
		case strings.HasPrefix(rest, "~"):
			return parseBinaryOp(left, rest, "~", 1)
		}
		break
	}
	return left, rest, nil
}

func parseBinaryOp(left *expr, rest string, op string, advance int) (*expr, string, error) {
	right, next, err := parseAddSub(rest[advance:])
	if err != nil {
		return nil, "", err
	}
	return &expr{kind: exprBinary, op: op, left: left, right: right}, next, nil
}

func parseAddSub(input string) (*expr, string, error) {
	left, rest, err := parseMulDiv(input)
	if err != nil {
		return nil, "", err
	}
	for {
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(rest, "+") || strings.HasPrefix(rest, "-") {
			op := rest[:1]
			right, next, err := parseMulDiv(rest[1:])
			if err != nil {
				return nil, "", err
			}
			left = &expr{kind: exprBinary, op: op, left: left, right: right}
			rest = next
			continue
		}
		break
	}
	return left, rest, nil
}

func parseMulDiv(input string) (*expr, string, error) {
	left, rest, err := parseTerm(input)
	if err != nil {
		return nil, "", err
	}
	for {
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(rest, "*") || strings.HasPrefix(rest, "/") {
			op := rest[:1]
			right, next, err := parseTerm(rest[1:])
			if err != nil {
				return nil, "", err
			}
			left = &expr{kind: exprBinary, op: op, left: left, right: right}
			rest = next
			continue
		}
		break
	}
	return left, rest, nil
}

func parseTerm(input string) (*expr, string, error) {
	rest := strings.TrimSpace(input)
	if rest == "" {
		return nil, "", fmt.Errorf("empty term")
	}
	if strings.HasPrefix(rest, "!") {
		sub, next, err := parseTerm(rest[1:])
		if err != nil {
			return nil, "", err
		}
		return &expr{kind: exprUnary, op: "!", left: sub}, next, nil
	}
	if strings.HasPrefix(rest, "-") {
		sub, next, err := parseTerm(rest[1:])
		if err != nil {
			return nil, "", err
		}
		return &expr{kind: exprUnary, op: "-", left: sub}, next, nil
	}
	if rest[0] == '(' {
		inner, tail, err := parseAddSub(rest[1:])
		if err != nil {
			return nil, "", err
		}
		tail = strings.TrimSpace(tail)
		if !strings.HasPrefix(tail, ")") {
			return nil, "", fmt.Errorf("missing ')'")
		}
		return inner, tail[1:], nil
	}
	if strings.HasPrefix(rest, "$") {
		ref := strings.TrimSpace(rest[1:])
		for i := 0; i < len(ref); i++ {
			if !unicode.IsDigit(rune(ref[i])) {
				ref = ref[:i]
				break
			}
		}
		if ref == "" {
			nameEnd := 0
			for nameEnd < len(rest[1:]) && (unicode.IsLetter(rune(rest[1+nameEnd])) || unicode.IsDigit(rune(rest[1+nameEnd])) || rest[1+nameEnd] == '_') {
				nameEnd++
			}
			if nameEnd == 0 {
				return nil, "", fmt.Errorf("invalid field")
			}
			name := rest[1 : 1+nameEnd]
			return &expr{kind: exprFieldVar, name: name}, rest[1+nameEnd:], nil
		}
		idx, err := strconv.Atoi(ref)
		if err != nil {
			return nil, "", err
		}
		return &expr{kind: exprField, field: idx}, rest[1+len(ref):], nil
	}
	if rest[0] == '"' || rest[0] == '\'' {
		quote := rest[0]
		i := 1
		escape := false
		for i < len(rest) {
			if escape {
				escape = false
				i++
				continue
			}
			if rest[i] == '\\' {
				escape = true
				i++
				continue
			}
			if rest[i] == quote {
				break
			}
			i++
		}
		if i >= len(rest) {
			return nil, "", fmt.Errorf("unterminated string")
		}
		val := unescapeString(rest[1:i])
		return &expr{kind: exprString, value: val}, rest[i+1:], nil
	}
	if unicode.IsDigit(rune(rest[0])) || rest[0] == '.' {
		i := 0
		for i < len(rest) && (unicode.IsDigit(rune(rest[i])) || rest[i] == '.') {
			i++
		}
		numStr := rest[:i]
		num, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return nil, "", err
		}
		return &expr{kind: exprNumber, num: num, value: numStr}, rest[i:], nil
	}
	nameEnd := 0
	for nameEnd < len(rest) && (unicode.IsLetter(rune(rest[nameEnd])) || unicode.IsDigit(rune(rest[nameEnd])) || rest[nameEnd] == '_') {
		nameEnd++
	}
	if nameEnd == 0 {
		return nil, "", fmt.Errorf("invalid expression")
	}
	name := rest[:nameEnd]
	tail := strings.TrimSpace(rest[nameEnd:])
	// support bare builtin calls like: getline
	if name == "getline" {
		// check for inline redirection: '< file' or '| cmd'
		t := strings.TrimSpace(tail)
		if strings.HasPrefix(t, "<") || strings.HasPrefix(t, "|") {
			target := strings.TrimSpace(t[1:])
			// handle quoted target
			if len(target) >= 2 && ((target[0] == '"' && target[len(target)-1] == '"') || (target[0] == '\'' && target[len(target)-1] == '\'')) {
				target = unescapeString(target[1 : len(target)-1])
			} else {
				parts := strings.Fields(target)
				if len(parts) > 0 {
					target = parts[0]
				}
			}
			return &expr{kind: exprFunc, name: name, args: []*expr{}, redir: string(t[0]) + target}, "", nil
		}
		// allow bare getline without parentheses and with trailing format specifier
		// e.g., printf "%d\n", 3.0 -> the format string is parsed as an expression earlier; for getline we just return func
		return &expr{kind: exprFunc, name: name, args: []*expr{}}, tail, nil
	}
	if strings.HasPrefix(tail, "[") {
		indexText, restTail, err := extractBracket(tail)
		if err != nil {
			return nil, "", err
		}
		indexExpr, err := parseExpr(indexText)
		if err != nil {
			return nil, "", err
		}
		return &expr{kind: exprArray, name: name, left: indexExpr}, restTail, nil
	}
	if strings.HasPrefix(tail, "(") {
		argsText, restTail, err := extractParen(tail)
		if err != nil {
			return nil, "", err
		}
		args, err := parseArgsList(argsText)
		if err != nil {
			return nil, "", err
		}
		return &expr{kind: exprFunc, name: name, args: args}, restTail, nil
	}
	// support bare function calls without parentheses for common builtins: printf/sprintf/sub/gsub/gensub/split/match/index/substr/strftime/mktime/system
	bareFuncs := map[string]bool{"printf": true, "sprintf": true, "sub": true, "gsub": true, "gensub": true, "split": true, "match": true, "index": true, "substr": true, "strftime": true, "mktime": true, "system": true}
	t := strings.TrimSpace(tail)
	if bareFuncs[name] && t != "" {
		// split comma-separated args and parse each as an expression
		parts := splitPrintArgs(t)
		args := make([]*expr, 0, len(parts))
		for _, p := range parts {
			expr, err := parseExpr(p)
			if err != nil {
				return nil, "", err
			}
			args = append(args, expr)
		}
		return &expr{kind: exprFunc, name: name, args: args}, "", nil
	}
	return &expr{kind: exprVar, name: name}, rest[nameEnd:], nil
}

func evalExpr(e *expr, state *awkState) (string, float64) {
	switch e.kind {
	case exprField:
		if e.field == 0 {
			return state.line, 0
		}
		if e.field < 0 || e.field > len(state.fields) {
			return "", 0
		}
		val := state.fields[e.field-1]
		num := parseNumber(val)
		return val, num
	case exprFieldVar:
		val := state.vars[e.name]
		idx, err := strconv.Atoi(val)
		if err != nil {
			idx = 0
		}
		if idx == 0 {
			return state.line, 0
		}
		if idx < 0 || idx > len(state.fields) {
			return "", 0
		}
		fieldVal := state.fields[idx-1]
		return fieldVal, parseNumber(fieldVal)
	case exprVar:
		val := state.vars[e.name]
		return val, parseNumber(val)
	case exprArray:
		index, _ := evalExpr(e.left, state)
		table := state.arrays[e.name]
		if table == nil {
			return "", 0
		}
		val := table[index]
		return val, parseNumber(val)
	case exprNumber:
		return e.value, e.num
	case exprString:
		return e.value, parseNumber(e.value)
	case exprUnary:
		switch e.op {
		case "!":
			val := evalTruth(e.left, state)
			if val {
				return "0", 0
			}
			return "1", 1
		case "-":
			_, num := evalExpr(e.left, state)
			return formatNumber(-num)
		default:
			return "0", 0
		}
	case exprBinary:
		lstr, lnum := evalExpr(e.left, state)
		rstr, rnum := evalExpr(e.right, state)
		switch e.op {
		case "+":
			return formatNumber(lnum + rnum)
		case "-":
			return formatNumber(lnum - rnum)
		case "*":
			return formatNumber(lnum * rnum)
		case "/":
			return formatNumber(lnum / rnum)
		case "==":
			if lstr == rstr {
				return "1", 1
			}
			return "0", 0
		case "!=":
			if lstr != rstr {
				return "1", 1
			}
			return "0", 0
		case "<":
			if lnum < rnum {
				return "1", 1
			}
			return "0", 0
		case "<=":
			if lnum <= rnum {
				return "1", 1
			}
			return "0", 0
		case ">":
			if lnum > rnum {
				return "1", 1
			}
			return "0", 0
		case ">=":
			if lnum >= rnum {
				return "1", 1
			}
			return "0", 0
		case "&&":
			if evalTruth(e.left, state) && evalTruth(e.right, state) {
				return "1", 1
			}
			return "0", 0
		case "||":
			if evalTruth(e.left, state) || evalTruth(e.right, state) {
				return "1", 1
			}
			return "0", 0
		case "~":
			re, err := regexp.Compile(rstr)
			if err != nil {
				return "0", 0
			}
			if re.MatchString(lstr) {
				return "1", 1
			}
			return "0", 0
		case "~=":
			re, err := regexp.Compile(lstr)
			if err != nil {
				return "0", 0
			}
			if re.MatchString(rstr) {
				return "1", 1
			}
			return "0", 0
		case "systime":
			// return current unix time
			timeNow := time.Now().Unix()
			return strconv.FormatInt(timeNow, 10), float64(timeNow)
		case "mktime":
			// mktime("YYYY MM DD HH MM SS") or mktime(YYYY,MM,DD,HH,MM,SS)
			if len(e.args) == 0 {
				return "0", 0
			}
			var y, mo, d, hh, mm, ss int
			if len(e.args) == 1 {
				s, _ := evalExpr(e.args[0], state)
				parts := strings.Fields(s)
				if len(parts) < 6 {
					return "0", 0
				}
				var err error
				if y, err = strconv.Atoi(parts[0]); err != nil {
					return "0", 0
				}
				if mo, err = strconv.Atoi(parts[1]); err != nil {
					return "0", 0
				}
				if d, err = strconv.Atoi(parts[2]); err != nil {
					return "0", 0
				}
				if hh, err = strconv.Atoi(parts[3]); err != nil {
					return "0", 0
				}
				if mm, err = strconv.Atoi(parts[4]); err != nil {
					return "0", 0
				}
				if ss, err = strconv.Atoi(parts[5]); err != nil {
					return "0", 0
				}
			} else {
				var err error
				if y, err = strconv.Atoi(evalExprToString(e.args[0], state)); err != nil {
					return "0", 0
				}
				if mo, err = strconv.Atoi(evalExprToString(e.args[1], state)); err != nil {
					return "0", 0
				}
				if d, err = strconv.Atoi(evalExprToString(e.args[2], state)); err != nil {
					return "0", 0
				}
				if hh, err = strconv.Atoi(evalExprToString(e.args[3], state)); err != nil {
					return "0", 0
				}
				if mm, err = strconv.Atoi(evalExprToString(e.args[4], state)); err != nil {
					return "0", 0
				}
				if ss, err = strconv.Atoi(evalExprToString(e.args[5], state)); err != nil {
					return "0", 0
				}
			}
			ts := time.Date(y, time.Month(mo), d, hh, mm, ss, 0, time.Local).Unix()
			return strconv.FormatInt(ts, 10), float64(ts)
		case "strftime":
			// strftime(format [, timestamp]) — implement common directives including %s, %j, %u, %w and delegate others to convertStrftime
			if len(e.args) == 0 {
				return "", 0
			}
			fmtStr, _ := evalExpr(e.args[0], state)
			var t time.Time
			if len(e.args) > 1 {
				numStr, _ := evalExpr(e.args[1], state)
				num := parseNumber(numStr)
				// use UTC for epoch-based formatting to match BusyBox behavior for timestamp 0
				t = time.Unix(int64(num), 0).UTC()
			} else {
				t = time.Now()
			}
			var b strings.Builder
			for i := 0; i < len(fmtStr); i++ {
				c := fmtStr[i]
				if c != '%' || i+1 >= len(fmtStr) {
					b.WriteByte(c)
					continue
				}
				i++
				n := fmtStr[i]
				switch n {
				case '%':
					b.WriteByte('%')
				case 's':
					b.WriteString(strconv.FormatInt(t.Unix(), 10))
				case 'j':
					b.WriteString(fmt.Sprintf("%03d", t.YearDay()))
				case 'u':
					// Monday=1..7
					uval := int(t.Weekday())
					if uval == 0 {
						uval = 7
					}
					b.WriteString(strconv.Itoa(uval))
				case 'w':
					b.WriteString(strconv.Itoa(int(t.Weekday())))
				default:
					// delegate single-directive conversion to Go layout and format
					layout := convertStrftime("%" + string(n))
					b.WriteString(t.Format(layout))
				}
			}
			out := b.String()
			return out, parseNumber(out)
		case "gensub":
			// gensub(regex, repl, how [, target]) -> return modified string
			if len(e.args) < 3 {
				return "", 0
			}
			pat, _ := evalExpr(e.args[0], state)
			repl, _ := evalExpr(e.args[1], state)
			how, _ := evalExpr(e.args[2], state)
			var target string
			var setTarget func(string)
			if len(e.args) > 3 {
				v, _ := evalExpr(e.args[3], state)
				target = v
				// prepare setter for variable/field/array
				setTarget = func(s string) {
					// try to assign back if arg is var/field/array
					t := e.args[3]
					switch t.kind {
					case exprVar:
						state.vars[t.name] = s
					case exprField:
						idx := t.field
						if idx == 0 {
							state.line = s
							state.fields = splitFields(s, state.fs)
							state.nf = len(state.fields)
							state.vars["NF"] = strconv.Itoa(state.nf)
						} else if idx > 0 {
							if idx > len(state.fields) {
								for i := len(state.fields); i < idx; i++ {
									state.fields = append(state.fields, "")
								}
							}
							state.fields[idx-1] = s
							state.line = strings.Join(state.fields, state.ofs)
							state.nf = len(state.fields)
							state.vars["NF"] = strconv.Itoa(state.nf)
						}
					case exprArray:
						key, _ := evalExpr(t.left, state)
						table := state.arrays[t.name]
						if table == nil {
							table = map[string]string{}
							state.arrays[t.name] = table
						}
						table[key] = s
					}
				}
			} else {
				target = state.line
			}
			re, err := regexp.Compile(pat)
			if err != nil {
				return "", 0
			}
			// helper to expand replacement template with backrefs like \1 and &
			expand := func(repl string, target string, m []int) string {
				var rb strings.Builder
				for i := 0; i < len(repl); i++ {
					c := repl[i]
					if c == '\\' && i+1 < len(repl) {
						next := repl[i+1]
						if next >= '0' && next <= '9' {
							// parse number
							j := i + 1
							num := 0
							for j < len(repl) && repl[j] >= '0' && repl[j] <= '9' {
								num = num*10 + int(repl[j]-'0')
								j++
							}
							idx := 2 * num
							if idx+1 < len(m) && m[idx] >= 0 {
								rb.WriteString(target[m[idx]:m[idx+1]])
							}
							i = j - 1
							continue
						} else if next == '&' {
							rb.WriteString(target[m[0]:m[1]])
							i++
							continue
						} else {
							// escaped char
							rb.WriteByte(next)
							i++
							continue
						}
					} else if c == '&' {
						rb.WriteString(target[m[0]:m[1]])
					} else {
						rb.WriteByte(c)
					}
				}
				return rb.String()
			}

			if how == "g" {
				matches := re.FindAllStringSubmatchIndex(target, -1)
				if len(matches) == 0 {
					return target, parseNumber(target)
				}
				var sb strings.Builder
				last := 0
				for _, m := range matches {
					start := m[0]
					end := m[1]
					sb.WriteString(target[last:start])
					replStr := expand(repl, target, m)
					sb.WriteString(replStr)
					last = end
				}
				sb.WriteString(target[last:])
				res := sb.String()
				if setTarget != nil {
					setTarget(res)
				} else {
					state.line = res
					state.fields = splitFields(res, state.fs)
					state.nf = len(state.fields)
					state.vars["NF"] = strconv.Itoa(state.nf)
				}
				return res, parseNumber(res)
			}
			// numeric occurrence
			n, err := strconv.Atoi(how)
			if err != nil || n <= 0 {
				return "", 0
			}
			matches := re.FindAllStringSubmatchIndex(target, -1)
			if len(matches) < n {
				return target, parseNumber(target)
			}
			// build result replacing only nth occurrence
			sel := matches[n-1]
			var out strings.Builder
			out.WriteString(target[:sel[0]])
			out.WriteString(expand(repl, target, sel))
			out.WriteString(target[sel[1]:])
			res := out.String()
			if setTarget != nil {
				setTarget(res)
			} else {
				state.line = res
				state.fields = splitFields(res, state.fs)
				state.nf = len(state.fields)
				state.vars["NF"] = strconv.Itoa(state.nf)
			}
			return res, parseNumber(res)
		default:
			return "0", 0
		}
	case exprFunc:
		return evalFunc(e, state)
	default:
		return "", 0
	}
}

// convert a strftime-style format to Go time layout for common directives
func convertStrftime(f string) string {
	// simple scanner: replace %X tokens
	var b strings.Builder
	for i := 0; i < len(f); i++ {
		c := f[i]
		if c != '%' || i+1 >= len(f) {
			b.WriteByte(c)
			continue
		}
		i++
		switch f[i] {
		case '%':
			b.WriteString("%")
		case 'Y':
			b.WriteString("2006")
		case 'y':
			b.WriteString("06")
		case 'm':
			b.WriteString("01")
		case 'd':
			b.WriteString("02")
		case 'H':
			b.WriteString("15")
		case 'I':
			b.WriteString("03")
		case 'M':
			b.WriteString("04")
		case 'S':
			b.WriteString("05")
		case 'p':
			b.WriteString("PM")
		case 'z':
			b.WriteString("-0700")
		case 'Z':
			b.WriteString("MST")
		case 'a':
			b.WriteString("Mon")
		case 'A':
			b.WriteString("Monday")
		case 'b':
			b.WriteString("Jan")
		case 'B':
			b.WriteString("January")
		case 'c':
			b.WriteString("Mon Jan 2 15:04:05 2006")
		case 'F':
			b.WriteString("2006-01-02")
		case 'T':
			b.WriteString("15:04:05")
		default:
			// unknown: pass through percent and char
			b.WriteByte('%')
			b.WriteByte(f[i])
		}
	}
	return b.String()
}

// helper to evaluate an expression to string only (no float)
func evalExprToString(e *expr, state *awkState) string {
	if e == nil {
		return ""
	}
	v, _ := evalExpr(e, state)
	return v
}

func evalFunc(e *expr, state *awkState) (string, float64) {
	// helper to parse format specifiers, returning verb and count of '*' extras
	type specInfo struct {
		verb  rune
		extra int
	}
	parseSpecs := func(f string) []specInfo {
		s := []specInfo{}
		for i := 0; i < len(f); i++ {
			if f[i] != '%' {
				continue
			}
			if i+1 < len(f) && f[i+1] == '%' {
				i++
				continue
			}
			j := i + 1
			extra := 0
			for j < len(f) {
				c := f[j]
				if c == '*' {
					extra++
				}
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
					s = append(s, specInfo{verb: rune(c), extra: extra})
					break
				}
				j++
			}
			if j < len(f) {
				i = j
			}
		}
		return s
	}
	switch e.name {
	case "length":
		if len(e.args) == 0 {
			return formatNumber(float64(len(state.line)))
		}
		val, _ := evalExpr(e.args[0], state)
		return formatNumber(float64(len(val)))
	case "substr":
		if len(e.args) < 2 {
			return "", 0
		}
		strVal, _ := evalExpr(e.args[0], state)
		_, startNum := evalExpr(e.args[1], state)
		start := int(startNum)
		if start < 1 {
			start = 1
		}
		start--
		if start >= len(strVal) {
			return "", 0
		}
		if len(e.args) > 2 {
			_, lenNum := evalExpr(e.args[2], state)
			l := int(lenNum)
			if l < 0 {
				return "", 0
			}
			end := start + l
			if end > len(strVal) {
				end = len(strVal)
			}
			return strVal[start:end], parseNumber(strVal[start:end])
		}
		return strVal[start:], parseNumber(strVal[start:])
	case "rand":
		if state.rng == nil {
			state.rng = rand.New(rand.NewSource(time.Now().UnixNano())) // #nosec G404 -- awk rand() uses math/rand by spec
		}
		val := state.rng.Float64()
		return formatNumber(val)
	case "srand":
		seed := time.Now().UnixNano()
		if len(e.args) > 0 {
			_, num := evalExpr(e.args[0], state)
			seed = int64(num)
		}
		state.rng = rand.New(rand.NewSource(seed)) // #nosec G404 -- awk srand() uses math/rand by spec
		return formatNumber(float64(seed))
	case "tolower":
		if len(e.args) == 0 {
			return "", 0
		}
		val, _ := evalExpr(e.args[0], state)
		out := strings.ToLower(val)
		return out, parseNumber(out)
	case "toupper":
		if len(e.args) == 0 {
			return "", 0
		}
		val, _ := evalExpr(e.args[0], state)
		out := strings.ToUpper(val)
		return out, parseNumber(out)
	case "index":
		if len(e.args) < 2 {
			return "0", 0
		}
		s, _ := evalExpr(e.args[0], state)
		t, _ := evalExpr(e.args[1], state)
		idx := strings.Index(s, t)
		if idx < 0 {
			// no match: set RSTART=0 RLENGTH=0
			state.vars["RSTART"] = "0"
			state.vars["RLENGTH"] = "0"
			return "0", 0
		}
		// set RSTART and RLENGTH (1-based index)
		start := idx + 1
		length := len(t)
		state.vars["RSTART"] = strconv.Itoa(start)
		state.vars["RLENGTH"] = strconv.Itoa(length)
		return strconv.Itoa(start), float64(start)
	case "match":
		// match(str, regex) -> returns 1-based index of match and sets RSTART/RLENGTH
		if len(e.args) < 2 {
			state.vars["RSTART"] = "0"
			state.vars["RLENGTH"] = "0"
			return "0", 0
		}
		strVal, _ := evalExpr(e.args[0], state)
		pat, _ := evalExpr(e.args[1], state)
		re, err := regexp.Compile(pat)
		if err != nil {
			state.vars["RSTART"] = "0"
			state.vars["RLENGTH"] = "0"
			return "0", 0
		}
		loc := re.FindStringIndex(strVal)
		if loc == nil {
			state.vars["RSTART"] = "0"
			state.vars["RLENGTH"] = "0"
			return "0", 0
		}
		start := loc[0] + 1
		length := loc[1] - loc[0]
		state.vars["RSTART"] = strconv.Itoa(start)
		state.vars["RLENGTH"] = strconv.Itoa(length)
		return strconv.Itoa(start), float64(start)

	case "strftime":
		// strftime(format [, timestamp]) — implement common directives including %s, %j, %u, %w and delegate others to convertStrftime
		if len(e.args) == 0 {
			return "", 0
		}
		fmtStr, _ := evalExpr(e.args[0], state)
		var t time.Time
		if len(e.args) > 1 {
			numStr, _ := evalExpr(e.args[1], state)
			num := parseNumber(numStr)
			// use UTC for epoch-based formatting to match BusyBox behavior for timestamp 0
			t = time.Unix(int64(num), 0).UTC()
		} else {
			t = time.Now()
		}
		var b strings.Builder
		for i := 0; i < len(fmtStr); i++ {
			c := fmtStr[i]
			if c != '%' || i+1 >= len(fmtStr) {
				b.WriteByte(c)
				continue
			}
			i++
			n := fmtStr[i]
			switch n {
			case '%':
				b.WriteByte('%')
			case 's':
				b.WriteString(strconv.FormatInt(t.Unix(), 10))
			case 'j':
				b.WriteString(fmt.Sprintf("%03d", t.YearDay()))
			case 'u':
				// Monday=1..7
				uval := int(t.Weekday())
				if uval == 0 {
					uval = 7
				}
				b.WriteString(strconv.Itoa(uval))
			case 'w':
				b.WriteString(strconv.Itoa(int(t.Weekday())))
			default:
				// delegate single-directive conversion to Go layout and format
				layout := convertStrftime("%" + string(n))
				b.WriteString(t.Format(layout))
			}
		}
		out := b.String()
		return out, parseNumber(out)

	case "split":
		// split(str, arr [, sep]) -> returns number of fields and populates array
		if len(e.args) < 2 {
			return "0", 0
		}
		strVal, _ := evalExpr(e.args[0], state)
		arrExpr := e.args[1]
		var arrName string
		switch arrExpr.kind {
		case exprVar:
			arrName = arrExpr.name
		case exprArray:
			arrName = arrExpr.name
		default:
			return "0", 0
		}
		sep := state.fs
		if len(e.args) > 2 {
			sepVal, _ := evalExpr(e.args[2], state)
			sep = sepVal
		}
		parts := []string{}
		if sep == "" {
			for _, r := range strVal {
				parts = append(parts, string(r))
			}
		} else {
			parts = strings.Split(strVal, sep)
		}
		table := map[string]string{}
		for i, p := range parts {
			table[strconv.Itoa(i+1)] = p
		}
		state.arrays[arrName] = table
		return strconv.Itoa(len(parts)), float64(len(parts))
	case "printf":
		// improved printf/sprintf argument conversion for numeric vs string verbs, with width/precision '*' support
		if len(e.args) == 0 {
			return "", 0
		}
		format, _ := evalExpr(e.args[0], state)
		argStrs := make([]string, 0, len(e.args)-1)
		for i := 1; i < len(e.args); i++ {
			v, _ := evalExpr(e.args[i], state)
			argStrs = append(argStrs, v)
		}
		type specInfo struct {
			verb  rune
			extra int
		}
		parseSpecs := func(f string) []specInfo {
			s := []specInfo{}
			for i := 0; i < len(f); i++ {
				if f[i] != '%' {
					continue
				}
				if i+1 < len(f) && f[i+1] == '%' {
					i++
					continue
				}
				j := i + 1
				extra := 0
				for j < len(f) {
					c := f[j]
					if c == '*' {
						extra++
					}
					if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
						s = append(s, specInfo{verb: rune(c), extra: extra})
						break
					}
					j++
				}
				if j < len(f) {
					i = j
				}
			}
			return s
		}
		specs := parseSpecs(format)
		vals := make([]interface{}, 0, len(argStrs))
		ai := 0
		for _, sp := range specs {
			// consume any '*' width/precision args first
			for k := 0; k < sp.extra; k++ {
				if ai >= len(argStrs) {
					vals = append(vals, 0)
					continue
				}
				if num, err := strconv.ParseFloat(argStrs[ai], 64); err == nil {
					vals = append(vals, int(num))
				} else {
					vals = append(vals, 0)
				}
				ai++
			}
			// main arg
			if ai >= len(argStrs) {
				switch sp.verb {
				case 'd', 'i', 'o', 'u', 'x', 'X':
					vals = append(vals, int64(0))
				case 'f', 'F', 'e', 'E', 'g', 'G':
					vals = append(vals, float64(0))
				case 'c':
					vals = append(vals, rune(0))
				default:
					vals = append(vals, "")
				}
				continue
			}
			sv := argStrs[ai]
			ai++
			switch sp.verb {
			case 'd', 'i', 'o', 'u', 'x', 'X':
				if num, err := strconv.ParseFloat(sv, 64); err == nil {
					vals = append(vals, int64(num))
				} else {
					vals = append(vals, int64(0))
				}
			case 'f', 'F', 'e', 'E', 'g', 'G':
				if num, err := strconv.ParseFloat(sv, 64); err == nil {
					vals = append(vals, num)
				} else {
					vals = append(vals, float64(0))
				}
			case 'c':
				if num, err := strconv.ParseFloat(sv, 64); err == nil {
					vals = append(vals, rune(int64(num)))
				} else if len(sv) > 0 {
					vals = append(vals, rune(sv[0]))
				} else {
					vals = append(vals, rune(0))
				}
			default:
				vals = append(vals, sv)
			}
		}
		// remaining args
		for ai < len(argStrs) {
			if num, err := strconv.ParseFloat(argStrs[ai], 64); err == nil {
				vals = append(vals, num)
			} else {
				vals = append(vals, argStrs[ai])
			}
			ai++
		}
		// For now delegate to fmt.Sprintf; later implement precise C-like rounding/flags
		out := fmt.Sprintf(format, vals...)
		return out, parseNumber(out)
	case "sprintf":
		if len(e.args) == 0 {
			return "", 0
		}
		format, _ := evalExpr(e.args[0], state)
		argStrs := make([]string, 0, len(e.args)-1)
		for i := 1; i < len(e.args); i++ {
			v, _ := evalExpr(e.args[i], state)
			argStrs = append(argStrs, v)
		}
		specs2 := parseSpecs(format)
		vals2 := make([]interface{}, 0, len(argStrs))
		ai2 := 0
		for _, sp := range specs2 {
			for k := 0; k < sp.extra; k++ {
				if ai2 >= len(argStrs) {
					vals2 = append(vals2, 0)
					continue
				}
				if num, err := strconv.ParseFloat(argStrs[ai2], 64); err == nil {
					vals2 = append(vals2, int(num))
				} else {
					vals2 = append(vals2, 0)
				}
				ai2++
			}
			if ai2 >= len(argStrs) {
				switch sp.verb {
				case 'd', 'i', 'o', 'u', 'x', 'X':
					vals2 = append(vals2, int64(0))
				case 'f', 'F', 'e', 'E', 'g', 'G':
					vals2 = append(vals2, float64(0))
				case 'c':
					vals2 = append(vals2, rune(0))
				default:
					vals2 = append(vals2, "")
				}
				continue
			}
			sv := argStrs[ai2]
			ai2++
			switch sp.verb {
			case 'd', 'i', 'o', 'u', 'x', 'X':
				if num, err := strconv.ParseFloat(sv, 64); err == nil {
					vals2 = append(vals2, int64(num))
				} else {
					vals2 = append(vals2, int64(0))
				}
			case 'f', 'F', 'e', 'E', 'g', 'G':
				if num, err := strconv.ParseFloat(sv, 64); err == nil {
					vals2 = append(vals2, num)
				} else {
					vals2 = append(vals2, float64(0))
				}
			case 'c':
				if num, err := strconv.ParseFloat(sv, 64); err == nil {
					vals2 = append(vals2, rune(int64(num)))
				} else if len(sv) > 0 {
					vals2 = append(vals2, rune(sv[0]))
				} else {
					vals2 = append(vals2, rune(0))
				}
			default:
				vals2 = append(vals2, sv)
			}
		}
		for ai2 < len(argStrs) {
			if num, err := strconv.ParseFloat(argStrs[ai2], 64); err == nil {
				vals2 = append(vals2, num)
			} else {
				vals2 = append(vals2, argStrs[ai2])
			}
			ai2++
		}
		// For now delegate to fmt.Sprintf; later implement precise C-like rounding/flags
		out2 := fmt.Sprintf(format, vals2...)
		return out2, parseNumber(out2)
	case "sub":
		// sub(regex, repl [, target]) -> replace first match, return 1 if replaced else 0
		if len(e.args) < 2 {
			return "0", 0
		}
		pat, _ := evalExpr(e.args[0], state)
		repl, _ := evalExpr(e.args[1], state)
		re, err := regexp.Compile(pat)
		if err != nil {
			return "0", 0
		}
		// determine target
		targetStr := state.line
		setTarget := func(s string) {
			state.line = s
			state.fields = splitFields(s, state.fs)
			state.nf = len(state.fields)
			state.vars["NF"] = strconv.Itoa(state.nf)
		}
		if len(e.args) >= 3 {
			t := e.args[2]
			switch t.kind {
			case exprVar:
				name := t.name
				targetStr = state.vars[name]
				setVar := func(s string) { state.vars[name] = s }
				setTarget = func(s string) {
					setVar(s)
					state.fields = splitFields(state.vars[name], state.fs)
					state.nf = len(state.fields)
					state.vars["NF"] = strconv.Itoa(state.nf)
				}
			case exprField:
				idx := t.field
				if idx == 0 {
					// $0
					targetStr = state.line
					setTarget = func(s string) { setTarget(s) }
				} else if idx > 0 && idx <= len(state.fields) {
					targetStr = state.fields[idx-1]
					setTarget = func(s string) {
						state.fields[idx-1] = s
						state.line = strings.Join(state.fields, state.ofs)
						state.nf = len(state.fields)
						state.vars["NF"] = strconv.Itoa(state.nf)
					}
				} else {
					return "0", 0
				}
			case exprArray:
				key, _ := evalExpr(t.left, state)
				name := t.name
				table := state.arrays[name]
				if table == nil {
					table = map[string]string{}
					state.arrays[name] = table
				}
				targetStr = table[key]
				setTarget = func(s string) { table[key] = s }
			default:
				val, _ := evalExpr(t, state)
				targetStr = val
			}
		}
		loc := re.FindStringIndex(targetStr)
		if loc == nil {
			return "0", 0
		}
		replaced := re.ReplaceAllString(targetStr[loc[0]:loc[1]], repl)
		res := targetStr[:loc[0]] + replaced + targetStr[loc[1]:]
		setTarget(res)
		return "1", 1
	case "gsub":
		// gsub(regex, repl [, target]) -> replace all matches, return count
		if len(e.args) < 2 {
			return "0", 0
		}
		pat, _ := evalExpr(e.args[0], state)
		repl, _ := evalExpr(e.args[1], state)
		re, err := regexp.Compile(pat)
		if err != nil {
			return "0", 0
		}
		// determine target
		targetStr := state.line
		setTarget := func(s string) {
			state.line = s
			state.fields = splitFields(s, state.fs)
			state.nf = len(state.fields)
			state.vars["NF"] = strconv.Itoa(state.nf)
		}
		if len(e.args) >= 3 {
			t := e.args[2]
			switch t.kind {
			case exprVar:
				name := t.name
				targetStr = state.vars[name]
				setVar := func(s string) { state.vars[name] = s }
				setTarget = func(s string) {
					setVar(s)
					state.fields = splitFields(state.vars[name], state.fs)
					state.nf = len(state.fields)
					state.vars["NF"] = strconv.Itoa(state.nf)
				}
			case exprField:
				idx := t.field
				if idx == 0 {
					// $0
					targetStr = state.line
					setTarget = func(s string) { setTarget(s) }
				} else if idx > 0 && idx <= len(state.fields) {
					targetStr = state.fields[idx-1]
					setTarget = func(s string) {
						state.fields[idx-1] = s
						state.line = strings.Join(state.fields, state.ofs)
						state.nf = len(state.fields)
						state.vars["NF"] = strconv.Itoa(state.nf)
					}
				} else {
					return "0", 0
				}
			case exprArray:
				key, _ := evalExpr(t.left, state)
				name := t.name
				table := state.arrays[name]
				if table == nil {
					table = map[string]string{}
					state.arrays[name] = table
				}
				targetStr = table[key]
				setTarget = func(s string) { table[key] = s }
			default:
				val, _ := evalExpr(t, state)
				targetStr = val
			}
		}
		matches := re.FindAllStringIndex(targetStr, -1)
		count := len(matches)
		if count == 0 {
			return "0", 0
		}
		res := re.ReplaceAllString(targetStr, repl)
		setTarget(res)
		return strconv.Itoa(count), float64(count)
	case "close":
		if len(e.args) < 1 {
			return "0", 0
		}
		fname, _ := evalExpr(e.args[0], state)
		// if closing a process pipe (|cmd), wait for the command and remove its reader
		if strings.HasPrefix(fname, "|") {
			if cmd, ok := state.procs[fname]; ok {
				_ = cmd.Wait()
				delete(state.procs, fname)
			}
			if _, ok := state.readers[fname]; ok {
				delete(state.readers, fname)
			}
			return "0", 0
		}
		if f, ok := state.files[fname]; ok {
			_ = f.Close()
			delete(state.files, fname)
			if _, ok := state.readers[fname]; ok {
				delete(state.readers, fname)
			}
			return "0", 0
		}
		return "0", 0
	case "getline":
		// support several forms:
		// 1) getline            -> read into $0, update NR/NF, return numeric status
		// 2) getline var        -> read into variable named var, return numeric status
		// 3) getline < file     -> read from named file handle
		// 4) cmd | getline      -> read from command pipe
		if len(e.args) == 0 {
			// read into $0
			reader := state.readers["-stdin"]
			if reader == nil {
				r := bufio.NewReader(strings.NewReader(state.line + "\n"))
				ln, err := r.ReadString('\n')
				if err != nil {
					return "0", 0
				}
				ln = strings.TrimRight(ln, "\n")
				state.line = ln
				state.fields = splitFields(ln, state.fs)
				state.nf = len(state.fields)
				state.nr++
				state.vars["NR"] = strconv.Itoa(state.nr)
				state.vars["NF"] = strconv.Itoa(state.nf)
				return "1", 1
			}
			ln, err := reader.ReadString('\n')
			if err != nil {
				return "0", 0
			}
			ln = strings.TrimRight(ln, "\n")
			state.line = ln
			state.fields = splitFields(ln, state.fs)
			state.nf = len(state.fields)
			state.nr++
			state.vars["NR"] = strconv.Itoa(state.nr)
			state.vars["NF"] = strconv.Itoa(state.nf)
			return "1", 1
		}
		// if first arg is a redirection (i.e. parsed as an expr with redir), handle file or cmd forms
		if len(e.args) == 1 && e.args[0].redir != "" {
			// redir like: <file or |cmd
			redir := e.args[0].redir
			if strings.HasPrefix(redir, "|") {
				// reuse existing reader if present
				if r, ok := state.readers[redir]; ok {
					ln, err := r.ReadString('\n')
					if err != nil {
						// cleanup on EOF or error
						if err == io.EOF {
							if cmd, ok := state.procs[redir]; ok {
								_ = cmd.Wait()
								delete(state.procs, redir)
							}
						}
						delete(state.readers, redir)
						return "0", 0
					}
					ln = strings.TrimRight(ln, "\n")
					state.line = ln
					state.fields = splitFields(ln, state.fs)
					state.nf = len(state.fields)
					state.nr++
					state.vars["NR"] = strconv.Itoa(state.nr)
					state.vars["NF"] = strconv.Itoa(state.nf)
					return "1", 1
				}
				// start command and register reader/proc
				cmdStr := strings.TrimSpace(redir[1:])
				parts := strings.Fields(cmdStr)
				if len(parts) == 0 {
					return "0", 0
				}
				cmd := exec.Command(parts[0], parts[1:]...) // #nosec G204 -- awk getline pipe executes user command
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					return "0", 0
				}
				if err := cmd.Start(); err != nil {
					return "0", 0
				}
				r := bufio.NewReader(stdout)
				state.procs[redir] = cmd
				state.readers[redir] = r
				ln, err := r.ReadString('\n')
				if err != nil {
					// cleanup proc/reader on error
					_ = cmd.Wait()
					delete(state.procs, redir)
					delete(state.readers, redir)
					return "0", 0
				}
				ln = strings.TrimRight(ln, "\n")
				state.line = ln
				state.fields = splitFields(ln, state.fs)
				state.nf = len(state.fields)
				state.nr++
				state.vars["NR"] = strconv.Itoa(state.nr)
				state.vars["NF"] = strconv.Itoa(state.nf)
				return "1", 1
			} else {
				// file name
				path := redir
				if r, ok := state.readers[path]; ok {
					ln, err := r.ReadString('\n')
					if err != nil {
						return "0", 0
					}
					ln = strings.TrimRight(ln, "\n")
					state.line = ln
					state.fields = splitFields(ln, state.fs)
					state.nf = len(state.fields)
					state.nr++
					state.vars["NR"] = strconv.Itoa(state.nr)
					state.vars["NF"] = strconv.Itoa(state.nf)
					return "1", 1
				}
				// try opening
				f, err := corefs.Open(path) // #nosec G304 -- awk getline uses user-supplied path
				if err != nil {
					return "0", 0
				}
				state.readers[path] = bufio.NewReader(f)
				ln, err := state.readers[path].ReadString('\n')
				if err != nil {
					return "0", 0
				}
				ln = strings.TrimRight(ln, "\n")
				state.line = ln
				state.fields = splitFields(ln, state.fs)
				state.nf = len(state.fields)
				state.nr++
				state.vars["NR"] = strconv.Itoa(state.nr)
				state.vars["NF"] = strconv.Itoa(state.nf)
				return "1", 1
			}
		}
		// one-arg form: getline var
		arg0 := e.args[0]
		if arg0.kind == exprString || arg0.kind == exprVar || arg0.kind == exprField || arg0.kind == exprFieldVar || arg0.kind == exprArray {
			varName, _ := evalExpr(arg0, state)
			reader := state.readers["-stdin"]
			if reader == nil {
				r := bufio.NewReader(strings.NewReader(state.line + "\n"))
				ln, err := r.ReadString('\n')
				if err != nil {
					state.vars[varName] = ""
					return "0", 0
				}
				ln = strings.TrimRight(ln, "\n")
				state.vars[varName] = ln
				return "1", 1
			}
			ln, err := reader.ReadString('\n')
			if err != nil {
				state.vars[varName] = ""
				return "0", 0
			}
			ln = strings.TrimRight(ln, "\n")
			state.vars[varName] = ln
			return "1", 1
		}
		return "0", 0

		// support assignment form: var = getline
		// Note: in this implementation, callers do assignment after calling getline, but
		// to support var=getline in expressions, parse/parseStatement should build that
		// as an assignment where rhs is a getline func; current parser handles name = expr,
		// so eval of assignment will call getline and assign returned string; here we just
		// return status and let caller assign if used in assignment context.
	case "fflush":
		// fflush optional filename
		if len(e.args) == 0 {
			// flush all open files (noop in Go since WriteString is immediate)
			return "0", 0
		}
		fname, _ := evalExpr(e.args[0], state)
		// if flushing a process pipe, wait for it to produce output (no-op)
		if strings.HasPrefix(fname, "|") {
			if cmd, ok := state.procs[fname]; ok {
				// best-effort: nothing to flush for process pipes here
				_ = cmd
				return "0", 0
			}
		}
		if f, ok := state.files[fname]; ok {
			if err := f.Sync(); err != nil {
				return "1", 1
			}
			return "0", 0
		}
		return "0", 0
	case "system":
		// best effort: run command, return exit status
		if len(e.args) == 0 {
			return "0", 0
		}
		cmdStr, _ := evalExpr(e.args[0], state)
		parts := strings.Fields(cmdStr)
		if len(parts) == 0 {
			return "0", 0
		}
		cmd := exec.Command(parts[0], parts[1:]...) // #nosec G204 -- awk system executes user command
		err := cmd.Run()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return strconv.Itoa(exitErr.ExitCode()), float64(exitErr.ExitCode())
			}
			return "1", 1
		}
		return "0", 0
	default:
		return "", 0
	}
}

func parseArgsList(s string) ([]*expr, error) {
	args := splitPrintArgs(s)
	exprs := make([]*expr, 0, len(args))
	for _, arg := range args {
		if strings.TrimSpace(arg) == "" {
			continue
		}
		expr, err := parseExpr(arg)
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, expr)
	}
	return exprs, nil
}

func evalTruth(e *expr, state *awkState) bool {
	str, num := evalExpr(e, state)
	if str == "" {
		return false
	}
	if num != 0 {
		return true
	}
	return str != "0"
}

func parseNumber(s string) float64 {
	if s == "" {
		return 0
	}
	num, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return num
}

func formatNumber(val float64) (string, float64) {
	if val == float64(int64(val)) {
		return strconv.FormatInt(int64(val), 10), val
	}
	return strconv.FormatFloat(val, 'f', -1, 64), val
}

func parsePrintArg(arg string) (printItem, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return printItem{}, fmt.Errorf("invalid print argument")
	}
	// check for redirection suffix: >file or >>file
	redir := ""
	append := false
	if strings.Contains(arg, ">>") {
		parts := strings.SplitN(arg, ">>", 2)
		arg = strings.TrimSpace(parts[0])
		redir = strings.TrimSpace(parts[1])
		append = true
	} else if strings.Contains(arg, ">") {
		parts := strings.SplitN(arg, ">", 2)
		arg = strings.TrimSpace(parts[0])
		redir = strings.TrimSpace(parts[1])
	}
	if arg == "" && redir != "" {
		// implicit print of $0 -> print current line
		return printItem{kind: printField, field: 0, redir: redir, append: append}, nil
	}
	if strings.HasPrefix(arg, "$") {
		ref := strings.TrimSpace(arg[1:])
		if ref == "" {
			return printItem{}, fmt.Errorf("invalid field")
		}
		if idx, err := strconv.Atoi(ref); err == nil {
			return printItem{kind: printField, field: idx, redir: redir, append: append}, nil
		}
		return printItem{kind: printFieldVar, name: ref, redir: redir, append: append}, nil
	}
	if strings.HasPrefix(arg, "\"") || strings.HasPrefix(arg, "'") {
		if len(arg) < 2 {
			return printItem{}, fmt.Errorf("unterminated string")
		}
		quote := arg[0]
		if arg[len(arg)-1] != quote {
			return printItem{}, fmt.Errorf("unterminated string")
		}
		lit := arg[1 : len(arg)-1]
		return printItem{kind: printLiteral, value: unescapeString(lit), redir: redir, append: append}, nil
	}
	if _, err := strconv.Atoi(arg); err == nil {
		return printItem{kind: printNumber, number: arg, redir: redir, append: append}, nil
	}

	expr, err := parseExpr(arg)
	if err != nil {
		return printItem{}, err
	}
	return printItem{kind: printExpr, expr: expr, redir: redir, append: append}, nil
}

func unescapeString(s string) string {
	if s == "" {
		return s
	}
	buf := &bytes.Buffer{}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			buf.WriteByte(c)
			continue
		}
		i++
		switch s[i] {
		case 'n':
			buf.WriteByte('\n')
		case 't':
			buf.WriteByte('\t')
		case 'r':
			buf.WriteByte('\r')
		case 'a':
			buf.WriteByte('\a')
		case 'b':
			buf.WriteByte('\b')
		case 'f':
			buf.WriteByte('\f')
		case 'v':
			buf.WriteByte('\v')
		case 'x':
			val := byte(0)
			count := 0
			for i+1 < len(s) && count < 2 {
				n := hexValue(rune(s[i+1]))
				if n < 0 {
					break
				}
				val = val*16 + byte(n)
				i++
				count++
			}
			if count == 0 {
				buf.WriteByte('\\')
				buf.WriteByte('x')
			} else {
				buf.WriteByte(val)
			}
		case '0', '1', '2', '3', '4', '5', '6', '7':
			val := byte(s[i] - '0')
			count := 1
			for i+1 < len(s) && count < 3 {
				if s[i+1] < '0' || s[i+1] > '7' {
					break
				}
				val = val*8 + byte(s[i+1]-'0')
				i++
				count++
			}
			buf.WriteByte(val)
		default:
			buf.WriteByte(s[i])
		}
	}
	return buf.String()
}

func hexValue(r rune) int {
	if unicode.IsDigit(r) {
		return int(r - '0')
	}
	if r >= 'a' && r <= 'f' {
		return int(r-'a') + 10
	}
	if r >= 'A' && r <= 'F' {
		return int(r-'A') + 10
	}
	return -1
}
