// Package procutil provides helpers for /proc-based applets.
package procutil

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

type ProcInfo struct {
	PID  int
	Comm string
	Args string
	UID  string
	GID  string
}

type MatchOptions struct {
	UseArgs bool
	Exact   bool
	Invert  bool
	User    string
}

// ListProcesses returns basic process info for all numeric /proc entries.
func ListProcesses() []ProcInfo {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	procs := make([]ProcInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		procs = append(procs, ReadProc(pid))
	}
	return procs
}

func MatchProcs(patterns []string, opts MatchOptions) []ProcInfo {
	if len(patterns) == 0 {
		return nil
	}
	var matches []ProcInfo
	for _, proc := range ListProcesses() {
		target := proc.Comm
		if opts.UseArgs && proc.Args != "" {
			target = proc.Args
		}
		match := false
		for _, pattern := range patterns {
			if opts.Exact {
				if target == pattern {
					match = true
					break
				}
				continue
			}
			if strings.Contains(target, pattern) {
				match = true
				break
			}
		}
		if opts.User != "" {
			user := LookupUser(proc.UID)
			if opts.User != proc.UID && opts.User != user {
				match = false
			}
		}
		if opts.Invert {
			match = !match
		}
		if match {
			matches = append(matches, proc)
		}
	}
	return matches
}

func SortByPID(procs []ProcInfo) {
	sort.Slice(procs, func(i, j int) bool {
		return procs[i].PID < procs[j].PID
	})
}

func ReadProc(pid int) ProcInfo {
	info := ProcInfo{PID: pid}
	info.Comm = ReadComm(pid)
	info.Args = ReadCmdline(pid)
	info.UID, info.GID = ReadIDs(pid)
	return info
}

func ReadComm(pid int) string {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "comm"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func ReadCmdline(pid int) string {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil || len(data) == 0 {
		return ""
	}
	parts := strings.Split(string(data), "\x00")
	var args []string
	for _, part := range parts {
		if part != "" {
			args = append(args, part)
		}
	}
	return strings.Join(args, " ")
}

func ReadIDs(pid int) (string, string) {
	status, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if err != nil {
		return "", ""
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
	return uid, gid
}

func LookupUser(uid string) string {
	if uid == "" {
		return ""
	}
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return uid
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) >= 3 && parts[2] == uid {
			return parts[0]
		}
	}
	return uid
}

func ReadFirstLine(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	if line == "" {
		return "", os.ErrNotExist
	}
	return line, nil
}

func ReadLoginName() (string, error) {
	if uid, err := ReadFirstLine("/proc/self/loginuid"); err == nil && uid != "" && uid != "4294967295" {
		return LookupUser(uid), nil
	}
	return "", syscall.Errno(syscall.ENXIO)
}

func ParseSignal(arg string) (syscall.Signal, error) {
	arg = strings.TrimPrefix(arg, "-")
	if arg == "" {
		return 0, syscall.EINVAL
	}
	if num, err := strconv.Atoi(arg); err == nil {
		return syscall.Signal(num), nil
	}
	arg = strings.ToUpper(arg)
	if strings.HasPrefix(arg, "SIG") {
		arg = strings.TrimPrefix(arg, "SIG")
	}
	for sig, name := range SignalNames() {
		if name == arg {
			return sig, nil
		}
	}
	return 0, syscall.EINVAL
}

func SignalNames() map[syscall.Signal]string {
	return map[syscall.Signal]string{
		syscall.SIGHUP:  "HUP",
		syscall.SIGINT:  "INT",
		syscall.SIGQUIT: "QUIT",
		syscall.SIGILL:  "ILL",
		syscall.SIGTRAP: "TRAP",
		syscall.SIGABRT: "ABRT",
		syscall.SIGBUS:  "BUS",
		syscall.SIGFPE:  "FPE",
		syscall.SIGKILL: "KILL",
		syscall.SIGUSR1: "USR1",
		syscall.SIGSEGV: "SEGV",
		syscall.SIGUSR2: "USR2",
		syscall.SIGPIPE: "PIPE",
		syscall.SIGALRM: "ALRM",
		syscall.SIGTERM: "TERM",
	}
}
