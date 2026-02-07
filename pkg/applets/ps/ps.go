// Package ps implements a minimal ps command.
package ps

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/applets/procutil"
	"github.com/rcarmo/go-busybox/pkg/core"
)

type options struct {
	columns string
	threads bool
}

type procInfo struct {
	pid    int
	user   string
	group  string
	comm   string
	args   string
	ppid   int
	pgid   int
	sid    int
	ttyNr  int
	stat   string
	nice   int
	vszKB  int
	rssKB  int
	ttyStr string
}

func Run(stdio *core.Stdio, args []string) int {
	opts := options{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-T" {
			opts.threads = true
			continue
		}
		if arg == "-a" || arg == "-Z" {
			continue
		}
		if arg == "-o" {
			if i+1 >= len(args) {
				stdio.Errorf("ps: option requires an argument -- 'o'\n")
				stdio.Println("BusyBox v1.35.0 (Debian 1:1.35.0-4+b7) multi-call binary.")
				stdio.Println("Usage: ps [-o COL1,COL2=HEADER] [-T]")
				stdio.Println("Show list of processes")
				stdio.Println("\t-o COL1,COL2=HEADER\tSelect columns for display")
				stdio.Println("\t-T\t\t\tShow threads")
				return core.ExitFailure
			}
			i++
			opts.columns = args[i]
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return core.UsageError(stdio, "ps", "invalid option -- '"+strings.TrimPrefix(arg, "-")+"'")
		}
	}

	procs := listProcesses(opts.threads)
	if opts.columns != "" {
		return printCustom(stdio, procs, opts.columns)
	}

	stdio.Println(formatHeader(defaultColumns()))
	for _, p := range procs {
		stdio.Println(formatRow(defaultColumns(), p))
	}
	return core.ExitSuccess
}

func listProcesses(includeThreads bool) []procInfo {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var procs []procInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		info := readProc(pid, pid)
		procs = append(procs, info)
		if includeThreads {
			procs = append(procs, listThreads(pid)...)
		}
	}
	sort.Slice(procs, func(i, j int) bool {
		return procs[i].pid < procs[j].pid
	})
	return procs
}

func listThreads(pid int) []procInfo {
	taskDir := filepath.Join("/proc", strconv.Itoa(pid), "task")
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		return nil
	}
	var threads []procInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		tid, err := strconv.Atoi(entry.Name())
		if err != nil || tid == pid {
			continue
		}
		info := readProc(tid, pid)
		threads = append(threads, info)
	}
	return threads
}

func readProc(pid int, ownerPid int) procInfo {
	info := procInfo{pid: pid}
	status, uid, gid := readStatus(ownerPid)
	info.user = procutil.LookupUser(uid)
	info.group = lookupGroup(gid)
	info.comm = readComm(pid)
	info.args = readCmdline(pid)
	info.ppid, info.pgid, info.sid, info.ttyNr, info.stat, info.nice, info.vszKB, info.rssKB = readStat(pid)
	info.ttyStr = formatTTY(info.ttyNr)
	if status == "" {
		info.user = "?"
		info.group = "?"
	}
	return info
}

func readCmdline(pid int) string {
	return procutil.ReadCmdline(pid)
}

func readComm(pid int) string {
	return readCommAt(filepath.Join("/proc", strconv.Itoa(pid), "comm"))
}

func readCommAt(path string) string {
	data, err := os.ReadFile(path) // #nosec G304 -- path is /proc-derived
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readUser(pid int) string {
	status, uid, _ := readStatus(pid)
	if status == "" || uid == "" {
		return "?"
	}
	return procutil.LookupUser(uid)
}

func readStatus(pid int) (string, string, string) {
	status, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status")) // #nosec G304 -- /proc read
	if err != nil {
		return "", "", ""
	}
	scanner := bufio.NewScanner(strings.NewReader(string(status)))
	uid := ""
	gid := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Uid:") {
			fields := strings.Fields(line)
			if len(fields) > 1 {
				uid = fields[1]
			}
		}
		if strings.HasPrefix(line, "Gid:") {
			fields := strings.Fields(line)
			if len(fields) > 1 {
				gid = fields[1]
			}
		}
		if uid != "" && gid != "" {
			break
		}
	}
	return string(status), uid, gid
}

func lookupGroup(gid string) string {
	data, err := os.ReadFile("/etc/group") // #nosec G304 -- fixed system file
	if err != nil {
		return gid
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) >= 3 && parts[2] == gid {
			return parts[0]
		}
	}
	return gid
}

func readStat(pid int) (int, int, int, int, string, int, int, int) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat")) // #nosec G304 -- /proc read
	if err != nil {
		return 0, 0, 0, 0, "", 0, 0, 0
	}
	stat := strings.TrimSpace(string(data))
	closeIdx := strings.LastIndex(stat, ") ")
	if closeIdx == -1 {
		return 0, 0, 0, 0, "", 0, 0, 0
	}
	rest := strings.Fields(stat[closeIdx+2:])
	if len(rest) < 22 {
		return 0, 0, 0, 0, "", 0, 0, 0
	}
	ppid := parseInt(rest[1])
	pgid := parseInt(rest[2])
	sid := parseInt(rest[3])
	ttyNr := parseInt(rest[4])
	state := rest[0]
	nice := parseInt(rest[16])
	vsizeKB := parseInt(rest[20]) / 1024
	rssKB := parseInt(rest[21]) * int(os.Getpagesize()) / 1024
	statField := state
	if nice < 0 {
		statField += "<"
	} else if nice > 0 {
		statField += "N"
	}
	return ppid, pgid, sid, ttyNr, statField, nice, vsizeKB, rssKB
}

func parseInt(value string) int {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return int(parsed)
}

func formatTTY(ttyNr int) string {
	if ttyNr <= 0 {
		return "?"
	}
	dev := int64(ttyNr)
	major := (dev >> 8) & 0xfff
	minor := (dev & 0xff) | ((dev >> 12) & 0xfff00)
	return fmt.Sprintf("%d,%d", major, minor)
}

func printCustom(stdio *core.Stdio, procs []procInfo, columns string) int {
	cols, invalid := parseColumns(columns)
	if invalid != "" {
		stdio.Errorf("ps: bad -o argument '%s', supported arguments: user,group,comm,args,pid,ppid,pgid,nice,rgroup,ruser,tty,vsz,sid,stat,rss\n", invalid)
		return core.ExitFailure
	}
	stdio.Println(formatHeader(cols))
	for _, p := range procs {
		stdio.Println(formatRow(cols, p))
	}
	return core.ExitSuccess
}

type columnSpec struct {
	name   string
	header string
	value  func(procInfo) string
	width  int
	left   bool
}

func parseColumns(spec string) ([]columnSpec, string) {
	var cols []columnSpec
	parts := strings.Split(spec, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, spec
		}
		name := part
		header := ""
		if strings.Contains(part, "=") {
			bits := strings.SplitN(part, "=", 2)
			name = bits[0]
			header = bits[1]
		}
		col, ok := columnByName(strings.ToLower(name))
		if !ok {
			return nil, name
		}
		if header != "" {
			col.header = header
		}
		cols = append(cols, col)
	}
	return cols, ""
}

func columnByName(name string) (columnSpec, bool) {
	switch name {
	case "pid":
		return columnSpec{name: name, header: "PID", width: 5, left: false, value: func(p procInfo) string { return strconv.Itoa(p.pid) }}, true
	case "user":
		return columnSpec{name: name, header: "USER", width: 8, left: true, value: func(p procInfo) string { return p.user }}, true
	case "ruser":
		return columnSpec{name: name, header: "RUSER", width: 8, left: true, value: func(p procInfo) string { return p.user }}, true
	case "group":
		return columnSpec{name: name, header: "GROUP", width: 8, left: true, value: func(p procInfo) string { return p.group }}, true
	case "rgroup":
		return columnSpec{name: name, header: "RGROUP", width: 8, left: true, value: func(p procInfo) string { return p.group }}, true
	case "comm":
		return columnSpec{name: name, header: "COMMAND", value: func(p procInfo) string {
			if p.comm == "" {
				return commandDisplay(p)
			}
			return p.comm
		}}, true
	case "args":
		return columnSpec{name: name, header: "COMMAND", value: commandDisplay}, true
	case "ppid":
		return columnSpec{name: name, header: "PPID", width: 5, left: false, value: func(p procInfo) string { return strconv.Itoa(p.ppid) }}, true
	case "pgid":
		return columnSpec{name: name, header: "PGID", width: 5, left: false, value: func(p procInfo) string { return strconv.Itoa(p.pgid) }}, true
	case "sid":
		return columnSpec{name: name, header: "SID", width: 5, left: false, value: func(p procInfo) string { return strconv.Itoa(p.sid) }}, true
	case "tty":
		return columnSpec{name: name, header: "TT", width: 6, left: true, value: func(p procInfo) string { return p.ttyStr }}, true
	case "vsz":
		return columnSpec{name: name, header: "VSZ", width: 4, left: false, value: func(p procInfo) string { return formatSize(p.vszKB) }}, true
	case "rss":
		return columnSpec{name: name, header: "RSS", width: 4, left: false, value: func(p procInfo) string { return formatSize(p.rssKB) }}, true
	case "nice":
		return columnSpec{name: name, header: "NI", width: 5, left: false, value: func(p procInfo) string { return strconv.Itoa(p.nice) }}, true
	case "stat":
		return columnSpec{name: name, header: "STAT", width: 4, left: true, value: func(p procInfo) string { return p.stat }}, true
	default:
		return columnSpec{}, false
	}
}

func defaultColumns() []columnSpec {
	return []columnSpec{
		{
			name:   "pid",
			header: "PID",
			width:  5,
			left:   false,
			value: func(p procInfo) string {
				return strconv.Itoa(p.pid)
			},
		},
		{
			name:   "user",
			header: "USER",
			width:  8,
			left:   true,
			value: func(p procInfo) string {
				return p.user
			},
		},
		{
			name:   "args",
			header: "COMMAND",
			value:  commandDisplay,
		},
	}
}

func formatHeader(cols []columnSpec) string {
	headers := make([]string, len(cols))
	for i, col := range cols {
		headers[i] = col.header
	}
	return formatRowValues(cols, headers, true)
}

func formatRow(cols []columnSpec, p procInfo) string {
	values := make([]string, len(cols))
	for i, col := range cols {
		values[i] = col.value(p)
	}
	return formatRowValues(cols, values, false)
}

func formatRowValues(cols []columnSpec, values []string, header bool) string {
	parts := make([]string, len(cols))
	for i, col := range cols {
		value := values[i]
		if col.width > 0 {
			left := header || col.left
			if left {
				value = fmt.Sprintf("%-*s", col.width, value)
			} else {
				value = fmt.Sprintf("%*s", col.width, value)
			}
		}
		parts[i] = value
	}
	return strings.Join(parts, " ")
}

func commandDisplay(p procInfo) string {
	if p.args == "" {
		return p.comm
	}
	cmdline := p.args
	argv0 := firstArg(cmdline)
	base := filepath.Base(argv0)
	if base == "busybox" {
		if path, err := exec.LookPath("busybox"); err == nil {
			cmdline = strings.Replace(cmdline, argv0, path, 1)
			argv0 = path
			base = filepath.Base(argv0)
		}
	}
	compare := base
	if strings.HasPrefix(compare, "-") {
		compare = strings.TrimPrefix(compare, "-")
	}
	if p.comm == "" || p.comm == compare || strings.HasPrefix(compare, p.comm) {
		return cmdline
	}
	return fmt.Sprintf("{%s} %s", p.comm, cmdline)
}

func formatSize(kb int) string {
	if kb >= 1024*1024 {
		gb := float64(kb) / 1024.0 / 1024.0
		if gb >= 10 {
			return fmt.Sprintf("%dg", int(gb))
		}
		val := fmt.Sprintf("%.1f", gb)
		val = strings.TrimSuffix(val, ".0")
		return val + "g"
	}
	if kb >= 10240 {
		return fmt.Sprintf("%dm", kb/1024)
	}
	return strconv.Itoa(kb)
}

func firstArg(cmdline string) string {
	if cmdline == "" {
		return ""
	}
	fields := strings.Fields(cmdline)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
