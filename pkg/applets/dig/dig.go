// Package dig implements a minimal dig-compatible DNS lookup tool.
package dig

import (
	crand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rcarmo/go-busybox/pkg/core"
)

type options struct {
	qname       string
	qtype       uint16
	qtypeLabel  string
	server      string
	port        string
	useTCP      bool
	shortOutput bool
	reverse     bool
	ipv4Only    bool
	ipv6Only    bool
}

type dnsHeader struct {
	id      uint16
	flags   uint16
	qdcount uint16
	ancount uint16
	nscount uint16
	arcount uint16
}

type dnsQuestion struct {
	name   string
	qtype  uint16
	qclass uint16
}

type dnsRR struct {
	name        string
	rtype       uint16
	class       uint16
	ttl         uint32
	rdata       []byte
	rdataOffset int
}

type dnsMessage struct {
	header     dnsHeader
	questions  []dnsQuestion
	answers    []dnsRR
	authority  []dnsRR
	additional []dnsRR
	raw        []byte
}

const (
	dnsClassIN = 1
	typeA      = 1
	typeNS     = 2
	typeCNAME  = 5
	typeSOA    = 6
	typePTR    = 12
	typeMX     = 15
	typeTXT    = 16
	typeAAAA   = 28
	typeSRV    = 33
)

var typeNames = map[string]uint16{
	"A":     typeA,
	"NS":    typeNS,
	"CNAME": typeCNAME,
	"SOA":   typeSOA,
	"PTR":   typePTR,
	"MX":    typeMX,
	"TXT":   typeTXT,
	"AAAA":  typeAAAA,
	"SRV":   typeSRV,
}

// Run executes the dig command with the given arguments.
//
// Supported flags:
//
//	-x ADDR     Perform a reverse DNS lookup
//	-4          Use IPv4 only
//	-6          Use IPv6 only
//	-t TYPE     Query type (A, AAAA, MX, NS, CNAME, TXT, SOA, PTR, SRV, ANY)
//	-p PORT     Use non-standard port number
//	@SERVER     Specify the DNS server to query
//
// The first non-flag argument is the domain name to query. When no
// server is specified, the system resolver from /etc/resolv.conf is used.
func Run(stdio *core.Stdio, args []string) int {
	opts, code := parseArgs(stdio, args)
	if code != core.ExitSuccess {
		return code
	}

	if opts.reverse {
		if ip := net.ParseIP(opts.qname); ip != nil {
			opts.qname = reverseName(ip)
			opts.qtype = typePTR
			opts.qtypeLabel = "PTR"
		} else {
			stdio.Errorf("dig: invalid address: %s\n", opts.qname)
			return core.ExitFailure
		}
	}

	msg, err := lookup(opts)
	if err != nil {
		stdio.Errorf("dig: %v\n", err)
		return core.ExitFailure
	}

	if opts.shortOutput {
		printShort(stdio, msg, opts)
		return core.ExitSuccess
	}

	printMessage(stdio, msg, opts)
	return core.ExitSuccess
}

func parseArgs(stdio *core.Stdio, args []string) (options, int) {
	opts := options{qtype: typeA, qtypeLabel: "A", port: "53"}
	typeExplicit := false

	for len(args) > 0 {
		arg := args[0]
		if arg == "--" {
			args = args[1:]
			break
		}
		if strings.HasPrefix(arg, "@") {
			opts.server = arg[1:]
			args = args[1:]
			continue
		}
		if strings.HasPrefix(arg, "+") {
			switch strings.TrimPrefix(arg, "+") {
			case "short":
				opts.shortOutput = true
			case "tcp":
				opts.useTCP = true
			default:
				return opts, core.UsageError(stdio, "dig", "invalid option")
			}
			args = args[1:]
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			switch arg {
			case "-x":
				opts.reverse = true
				args = args[1:]
			case "-4":
				opts.ipv4Only = true
				args = args[1:]
			case "-6":
				opts.ipv6Only = true
				args = args[1:]
			case "-t":
				if len(args) < 2 {
					return opts, core.UsageError(stdio, "dig", "missing type")
				}
				qtype, label, ok := parseType(args[1])
				if !ok {
					return opts, core.UsageError(stdio, "dig", "unknown type")
				}
				opts.qtype = qtype
				opts.qtypeLabel = label
				typeExplicit = true
				args = args[2:]
			case "-p":
				if len(args) < 2 {
					return opts, core.UsageError(stdio, "dig", "missing port")
				}
				opts.port = args[1]
				args = args[2:]
			default:
				return opts, core.UsageError(stdio, "dig", "invalid option")
			}
			continue
		}
		if opts.qname == "" {
			opts.qname = arg
			args = args[1:]
			continue
		}
		if !typeExplicit {
			if qtype, label, ok := parseType(arg); ok {
				opts.qtype = qtype
				opts.qtypeLabel = label
				typeExplicit = true
				args = args[1:]
				continue
			}
		}
		return opts, core.UsageError(stdio, "dig", "invalid arguments")
	}

	if opts.qname == "" {
		return opts, core.UsageError(stdio, "dig", "missing name")
	}
	if opts.ipv4Only && opts.ipv6Only {
		return opts, core.UsageError(stdio, "dig", "cannot combine -4 and -6")
	}

	if opts.server == "" {
		srv, err := defaultServer()
		if err != nil {
			return opts, core.FileError(stdio, "dig", "/etc/resolv.conf", err)
		}
		opts.server = srv
	}
	return opts, core.ExitSuccess
}

func parseType(val string) (uint16, string, bool) {
	upper := strings.ToUpper(val)
	if t, ok := typeNames[upper]; ok {
		return t, upper, true
	}
	return 0, "", false
}

func defaultServer() (string, error) {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "nameserver" {
			return fields[1], nil
		}
	}
	return "", errors.New("no nameserver found")
}

func lookup(opts options) (*dnsMessage, error) {
	msg, err := buildQuery(opts)
	if err != nil {
		return nil, err
	}
	req, err := packMessage(msg)
	if err != nil {
		return nil, err
	}

	server := net.JoinHostPort(opts.server, opts.port)
	if opts.ipv4Only {
		return exchange("udp4", server, req, opts)
	}
	if opts.ipv6Only {
		return exchange("udp6", server, req, opts)
	}
	return exchange("udp", server, req, opts)
}

func exchange(network, addr string, req []byte, opts options) (*dnsMessage, error) {
	if opts.useTCP {
		return exchangeTCP(network, addr, req)
	}

	conn, err := net.DialTimeout(network, addr, 3*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return nil, err
	}

	if _, err := conn.Write(req); err != nil {
		return nil, err
	}

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return unpackMessage(buf[:n])
}

func exchangeTCP(network, addr string, req []byte) (*dnsMessage, error) {
	conn, err := net.DialTimeout("tcp"+networkSuffix(network), addr, 3*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return nil, err
	}

	if len(req) > int(^uint16(0)) {
		return nil, fmt.Errorf("dig: request too large")
	}
	length := make([]byte, 2)
	binary.BigEndian.PutUint16(length, uint16(len(req))) // #nosec G115 -- length checked above
	if _, err := conn.Write(append(length, req...)); err != nil {
		return nil, err
	}

	hdr := make([]byte, 2)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return nil, err
	}
	respLen := binary.BigEndian.Uint16(hdr)
	resp := make([]byte, respLen)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return nil, err
	}
	return unpackMessage(resp)
}

func networkSuffix(network string) string {
	if strings.HasSuffix(network, "4") {
		return "4"
	}
	if strings.HasSuffix(network, "6") {
		return "6"
	}
	return ""
}

func buildQuery(opts options) (*dnsMessage, error) {
	id, err := randomUint16()
	if err != nil {
		return nil, err
	}
	flags := uint16(0x0100)
	return &dnsMessage{
		header:    dnsHeader{id: id, flags: flags, qdcount: 1},
		questions: []dnsQuestion{{name: opts.qname, qtype: opts.qtype, qclass: dnsClassIN}},
	}, nil
}

func randomUint16() (uint16, error) {
	var buf [2]byte
	if _, err := crand.Read(buf[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(buf[:]), nil
}

func packMessage(msg *dnsMessage) ([]byte, error) {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint16(buf[0:2], msg.header.id)
	binary.BigEndian.PutUint16(buf[2:4], msg.header.flags)
	binary.BigEndian.PutUint16(buf[4:6], msg.header.qdcount)
	binary.BigEndian.PutUint16(buf[6:8], msg.header.ancount)
	binary.BigEndian.PutUint16(buf[8:10], msg.header.nscount)
	binary.BigEndian.PutUint16(buf[10:12], msg.header.arcount)

	for _, q := range msg.questions {
		name, err := packName(q.name)
		if err != nil {
			return nil, err
		}
		buf = append(buf, name...)
		tmp := make([]byte, 4)
		binary.BigEndian.PutUint16(tmp[0:2], q.qtype)
		binary.BigEndian.PutUint16(tmp[2:4], q.qclass)
		buf = append(buf, tmp...)
	}
	return buf, nil
}

func packName(name string) ([]byte, error) {
	if name == "." {
		return []byte{0}, nil
	}
	parts := strings.Split(strings.TrimSuffix(name, "."), ".")
	var buf []byte
	for _, part := range parts {
		if len(part) == 0 || len(part) > 63 {
			return nil, errors.New("invalid name")
		}
		buf = append(buf, byte(len(part)))
		buf = append(buf, []byte(part)...)
	}
	buf = append(buf, 0)
	return buf, nil
}

func unpackMessage(data []byte) (*dnsMessage, error) {
	if len(data) < 12 {
		return nil, errors.New("short response")
	}
	msg := &dnsMessage{raw: data}
	msg.header = dnsHeader{
		id:      binary.BigEndian.Uint16(data[0:2]),
		flags:   binary.BigEndian.Uint16(data[2:4]),
		qdcount: binary.BigEndian.Uint16(data[4:6]),
		ancount: binary.BigEndian.Uint16(data[6:8]),
		nscount: binary.BigEndian.Uint16(data[8:10]),
		arcount: binary.BigEndian.Uint16(data[10:12]),
	}
	offset := 12
	for i := 0; i < int(msg.header.qdcount); i++ {
		name, next, err := unpackName(data, offset)
		if err != nil {
			return nil, err
		}
		offset = next
		if offset+4 > len(data) {
			return nil, errors.New("short question")
		}
		qtype := binary.BigEndian.Uint16(data[offset : offset+2])
		qclass := binary.BigEndian.Uint16(data[offset+2 : offset+4])
		offset += 4
		msg.questions = append(msg.questions, dnsQuestion{name: name, qtype: qtype, qclass: qclass})
	}

	readRR := func(count uint16) ([]dnsRR, int, error) {
		rrs := make([]dnsRR, 0, count)
		off := offset
		for i := 0; i < int(count); i++ {
			name, next, err := unpackName(data, off)
			if err != nil {
				return nil, off, err
			}
			off = next
			if off+10 > len(data) {
				return nil, off, errors.New("short rr")
			}
			rtype := binary.BigEndian.Uint16(data[off : off+2])
			class := binary.BigEndian.Uint16(data[off+2 : off+4])
			ttl := binary.BigEndian.Uint32(data[off+4 : off+8])
			rdlen := binary.BigEndian.Uint16(data[off+8 : off+10])
			off += 10
			if off+int(rdlen) > len(data) {
				return nil, off, errors.New("short rdata")
			}
			rdataOffset := off
			rdata := data[off : off+int(rdlen)]
			off += int(rdlen)
			rrs = append(rrs, dnsRR{name: name, rtype: rtype, class: class, ttl: ttl, rdata: rdata, rdataOffset: rdataOffset})
		}
		return rrs, off, nil
	}

	var err error
	msg.answers, offset, err = readRR(msg.header.ancount)
	if err != nil {
		return nil, err
	}
	msg.authority, offset, err = readRR(msg.header.nscount)
	if err != nil {
		return nil, err
	}
	msg.additional, offset, err = readRR(msg.header.arcount)
	if err != nil {
		return nil, err
	}

	_ = offset
	return msg, nil
}

func unpackName(data []byte, offset int) (string, int, error) {
	var labels []string
	start := offset
	jumped := false
	for {
		if offset >= len(data) {
			return "", offset, errors.New("name out of range")
		}
		length := data[offset]
		if length == 0 {
			if !jumped {
				offset++
			}
			break
		}
		if length&0xC0 == 0xC0 {
			if offset+1 >= len(data) {
				return "", offset, errors.New("pointer out of range")
			}
			ptr := int(length&0x3F)<<8 | int(data[offset+1])
			if ptr >= len(data) {
				return "", offset, errors.New("pointer out of range")
			}
			if !jumped {
				start = offset + 2
			}
			offset = ptr
			jumped = true
			continue
		}
		offset++
		if offset+int(length) > len(data) {
			return "", offset, errors.New("label out of range")
		}
		labels = append(labels, string(data[offset:offset+int(length)]))
		offset += int(length)
	}
	name := strings.Join(labels, ".")
	if !jumped {
		return name, offset, nil
	}
	return name, start, nil
}

func printShort(stdio *core.Stdio, msg *dnsMessage, opts options) {
	for _, rr := range msg.answers {
		line := formatRRData(rr, msg, opts, true)
		if line != "" {
			stdio.Println(line)
		}
	}
}

func printMessage(stdio *core.Stdio, msg *dnsMessage, opts options) {
	stdio.Println(";; QUESTION SECTION:")
	for _, q := range msg.questions {
		stdio.Printf(";%s\t\tIN\t%s\n", q.name, opts.qtypeLabel)
	}
	stdio.Println()

	if len(msg.answers) > 0 {
		stdio.Println(";; ANSWER SECTION:")
		for _, rr := range msg.answers {
			stdio.Println(formatRR(rr, msg, opts))
		}
		stdio.Println()
	}

	if len(msg.authority) > 0 {
		stdio.Println(";; AUTHORITY SECTION:")
		for _, rr := range msg.authority {
			stdio.Println(formatRR(rr, msg, opts))
		}
		stdio.Println()
	}

	if len(msg.additional) > 0 {
		stdio.Println(";; ADDITIONAL SECTION:")
		for _, rr := range msg.additional {
			stdio.Println(formatRR(rr, msg, opts))
		}
		stdio.Println()
	}
}

func formatRR(rr dnsRR, msg *dnsMessage, opts options) string {
	data := formatRRData(rr, msg, opts, false)
	if data == "" {
		data = fmt.Sprintf("\\# %d", len(rr.rdata))
	}
	return fmt.Sprintf("%s\t%d\tIN\t%s\t%s", rr.name, rr.ttl, typeLabel(rr.rtype), data)
}

func formatRRData(rr dnsRR, msg *dnsMessage, opts options, short bool) string {
	switch rr.rtype {
	case typeA:
		if len(rr.rdata) != 4 {
			return ""
		}
		return net.IP(rr.rdata).String()
	case typeAAAA:
		if len(rr.rdata) != 16 {
			return ""
		}
		return net.IP(rr.rdata).String()
	case typeCNAME, typeNS, typePTR:
		name, _, err := unpackName(msg.raw, rr.rdataOffset)
		if err != nil {
			return ""
		}
		return name
	case typeMX:
		if len(rr.rdata) < 3 {
			return ""
		}
		pref := binary.BigEndian.Uint16(rr.rdata[0:2])
		name, _, err := unpackName(msg.raw, rr.rdataOffset+2)
		if err != nil {
			return ""
		}
		if short {
			return name
		}
		return fmt.Sprintf("%d %s", pref, name)
	case typeSOA:
		mname, off, err := unpackName(msg.raw, rr.rdataOffset)
		if err != nil {
			return ""
		}
		rname, off, err := unpackName(msg.raw, off)
		if err != nil {
			return ""
		}
		limit := rr.rdataOffset + len(rr.rdata)
		if off+20 > limit {
			return ""
		}
		serial := binary.BigEndian.Uint32(msg.raw[off : off+4])
		refresh := binary.BigEndian.Uint32(msg.raw[off+4 : off+8])
		retry := binary.BigEndian.Uint32(msg.raw[off+8 : off+12])
		expire := binary.BigEndian.Uint32(msg.raw[off+12 : off+16])
		minimum := binary.BigEndian.Uint32(msg.raw[off+16 : off+20])
		return fmt.Sprintf("%s %s %d %d %d %d %d", mname, rname, serial, refresh, retry, expire, minimum)
	case typeTXT:
		if len(rr.rdata) < 1 {
			return ""
		}
		length := int(rr.rdata[0])
		if len(rr.rdata) < 1+length {
			return ""
		}
		return fmt.Sprintf("\"%s\"", string(rr.rdata[1:1+length]))
	case typeSRV:
		if len(rr.rdata) < 7 {
			return ""
		}
		priority := binary.BigEndian.Uint16(rr.rdata[0:2])
		weight := binary.BigEndian.Uint16(rr.rdata[2:4])
		port := binary.BigEndian.Uint16(rr.rdata[4:6])
		target, _, err := unpackName(msg.raw, rr.rdataOffset+6)
		if err != nil {
			return ""
		}
		if short {
			return target
		}
		return fmt.Sprintf("%d %d %d %s", priority, weight, port, target)
	default:
		return ""
	}
}

func typeLabel(t uint16) string {
	for name, val := range typeNames {
		if val == t {
			return name
		}
	}
	return strconv.Itoa(int(t))
}

func reverseName(ip net.IP) string {
	if v4 := ip.To4(); v4 != nil {
		return fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa", v4[3], v4[2], v4[1], v4[0])
	}
	v6 := ip.To16()
	if v6 == nil {
		return ""
	}
	var parts []string
	for i := len(v6) - 1; i >= 0; i-- {
		b := v6[i]
		parts = append(parts, fmt.Sprintf("%x", b&0x0f))
		parts = append(parts, fmt.Sprintf("%x", (b>>4)&0x0f))
	}
	return strings.Join(parts, ".") + ".ip6.arpa"
}
