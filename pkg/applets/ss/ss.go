// Package ss implements a minimal /proc-based ss command.
package ss

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
)

type options struct {
	showTCP    bool
	showUDP    bool
	showListen bool
	numeric    bool
	showUsers  bool
	summary    bool
}

type socketEntry struct {
	netid string
	state string
	recvQ int
	sendQ int
	local string
	peer  string
	inode string
	user  string
}

type summaryCounts struct {
	total int
	tcp   int
	udp   int
}

func Run(stdio *core.Stdio, args []string) int {
	opts := options{}
	for _, arg := range args {
		if arg == "-s" {
			opts.summary = true
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, c := range arg[1:] {
				switch c {
				case 't':
					opts.showTCP = true
				case 'u':
					opts.showUDP = true
				case 'l':
					opts.showListen = true
				case 'n':
					opts.numeric = true
				case 'p':
					opts.showUsers = true
				case 's':
					opts.summary = true
				default:
					return core.UsageError(stdio, "ss", "invalid option -- '"+string(c)+"'")
				}
			}
		}
	}
	if !opts.showTCP && !opts.showUDP {
		opts.showTCP = true
		opts.showUDP = true
	}

	userMap := map[string]string{}
	if opts.showUsers {
		userMap = buildUserMap()
	}

	entries, counts := collectEntries(opts, userMap)
	if opts.summary {
		printSummary(stdio, counts)
		return core.ExitSuccess
	}

	stdio.Println("Netid State  Recv-Q Send-Q Local Address:Port  Peer Address:PortProcess")
	for _, e := range entries {
		user := ""
		if e.user != "" {
			user = e.user
		}
		stdio.Printf("%-5s %-6s %6d %6d %-20s %-20s%s\n", e.netid, e.state, e.recvQ, e.sendQ, e.local, e.peer, user)
	}
	return core.ExitSuccess
}

func collectEntries(opts options, userMap map[string]string) ([]socketEntry, summaryCounts) {
	entries := []socketEntry{}
	counts := summaryCounts{}
	if opts.showTCP {
		es := readProcNet("/proc/net/tcp", "tcp", opts, userMap)
		entries = append(entries, es...)
		counts.tcp += len(es)
		counts.total += len(es)
	}
	if opts.showUDP {
		es := readProcNet("/proc/net/udp", "udp", opts, userMap)
		entries = append(entries, es...)
		counts.udp += len(es)
		counts.total += len(es)
	}
	return entries, counts
}

func readProcNet(path string, netid string, opts options, userMap map[string]string) []socketEntry {
	if !strings.HasPrefix(path, "/proc/") {
		return nil
	}
	file, err := os.Open(path) // #nosec G304 -- reads fixed /proc paths
	if err != nil {
		return nil
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	first := true
	var entries []socketEntry
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if first {
			first = false
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		state := tcpState(fields[3])
		if opts.showListen && state != "LISTEN" {
			continue
		}
		sendQ, recvQ := parseQueue(fields[4])
		local := formatAddr(fields[1], netid, opts.numeric)
		peer := formatAddr(fields[2], netid, opts.numeric)
		inode := fields[9]
		user := ""
		if name, ok := userMap[inode]; ok {
			user = " " + name
		}
		entries = append(entries, socketEntry{
			netid: netid,
			state: state,
			recvQ: recvQ,
			sendQ: sendQ,
			local: local,
			peer:  peer,
			inode: inode,
			user:  user,
		})
	}
	return entries
}

func parseQueue(val string) (sendQ int, recvQ int) {
	parts := strings.Split(val, ":")
	if len(parts) != 2 {
		return 0, 0
	}
	sendQ, _ = parseHex(parts[0])
	recvQ, _ = parseHex(parts[1])
	return sendQ, recvQ
}

func formatAddr(val string, netid string, numeric bool) string {
	parts := strings.Split(val, ":")
	if len(parts) != 2 {
		return val
	}
	addrHex := parts[0]
	portHex := parts[1]
	port, _ := parseHex(portHex)
	ip := parseIPv4(addrHex)
	if ip == "" {
		ip = "0.0.0.0"
	}
	if !numeric {
		if host := resolveHost(ip); host != "" {
			ip = host
		}
		if svc := resolveService(port, netid); svc != "" {
			return fmt.Sprintf("%s:%s", ip, svc)
		}
	}
	return fmt.Sprintf("%s:%d", ip, port)
}

func parseIPv4(hexAddr string) string {
	if len(hexAddr) != 8 {
		return ""
	}
	b1, _ := parseHex(hexAddr[6:8])
	b2, _ := parseHex(hexAddr[4:6])
	b3, _ := parseHex(hexAddr[2:4])
	b4, _ := parseHex(hexAddr[0:2])
	return fmt.Sprintf("%d.%d.%d.%d", b1, b2, b3, b4)
}

func parseHex(val string) (int, error) {
	n, err := strconv.ParseInt(val, 16, 32)
	return int(n), err
}

func resolveHost(ip string) string {
	hosts, err := net.LookupAddr(ip)
	if err != nil || len(hosts) == 0 {
		return ""
	}
	return strings.TrimSuffix(hosts[0], ".")
}

func resolveService(port int, netid string) string {
	return ""
}

func tcpState(code string) string {
	switch code {
	case "01":
		return "ESTAB"
	case "0A":
		return "LISTEN"
	case "07":
		return "UNCONN"
	default:
		return "UNCONN"
	}
}

func buildUserMap() map[string]string {
	users := map[string]string{}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return users
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid := entry.Name()
		if _, err := strconv.Atoi(pid); err != nil {
			continue
		}
		fdDir := filepath.Join("/proc", pid, "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if strings.HasPrefix(link, "socket:[") {
				inode := strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")
				if _, ok := users[inode]; !ok {
					users[inode] = fmt.Sprintf("users:(\"%s\",pid=%s,fd=%s)", pid, pid, fd.Name())
				}
			}
		}
	}
	return users
}

func printSummary(stdio *core.Stdio, counts summaryCounts) {
	stdio.Printf("Total: %d\n", counts.total)
	stdio.Printf("TCP:   %d\n", counts.tcp)
	stdio.Printf("UDP:   %d\n", counts.udp)
}
