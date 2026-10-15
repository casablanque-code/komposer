package composer

import (
	"net"
	"strconv"
	"strings"
)

// portSpec is a parsed `ports:` entry in short syntax:
//
//	[[IP:][host[-end]:]]container[-end][/protocol]
//
// where IP is an IPv4 address or an IPv6 address in square brackets.
type portSpec struct {
	HostIP    string // "" if absent; IPv6 without the brackets
	Host      string // "" (not given / random), "8080" or "8000-8010"
	Container string // "80" or "8000-8010"
	Proto     string // "" or "/tcp", "/udp", "/sctp" exactly as written
}

// parsePortSpec parses a short-syntax port mapping. The second result is
// false if the entry is not valid short syntax.
func parsePortSpec(s string) (portSpec, bool) {
	var ps portSpec
	s = strings.TrimSpace(s)
	if s == "" {
		return ps, false
	}

	if i := strings.LastIndex(s, "/"); i >= 0 {
		switch strings.ToLower(s[i+1:]) {
		case "tcp", "udp", "sctp":
		default:
			return ps, false
		}
		ps.Proto = s[i:]
		s = s[:i]
	}

	var hostPart, containerPart string
	if strings.HasPrefix(s, "[") {
		// "[ipv6]:host:container" or "[ipv6]::container". The brackets
		// are required: unbracketed, an IPv6 address is ambiguous with
		// the port separators.
		end := strings.Index(s, "]")
		if end < 0 || end+1 >= len(s) || s[end+1] != ':' {
			return ps, false
		}
		ip := net.ParseIP(s[1:end])
		if ip == nil || ip.To4() != nil {
			return ps, false
		}
		ps.HostIP = s[1:end]
		parts := strings.Split(s[end+2:], ":")
		if len(parts) != 2 {
			return ps, false
		}
		hostPart, containerPart = parts[0], parts[1]
	} else {
		parts := strings.Split(s, ":")
		switch len(parts) {
		case 1:
			containerPart = parts[0]
		case 2:
			if parts[0] == "" {
				return ps, false
			}
			hostPart, containerPart = parts[0], parts[1]
		case 3:
			if net.ParseIP(parts[0]) == nil {
				return ps, false
			}
			ps.HostIP = parts[0]
			// The host segment of "ip:host:container" may be empty,
			// meaning "pick a random host port" — "127.0.0.1::8080".
			hostPart, containerPart = parts[1], parts[2]
		default:
			return ps, false
		}
	}

	cLo, cHi, cRange, ok := parsePortRange(containerPart)
	if !ok {
		return ps, false
	}
	if hostPart != "" {
		hLo, hHi, hRange, ok := parsePortRange(hostPart)
		if !ok {
			return ps, false
		}
		// A container range has to be matched by a host range of the
		// same length; a host range may also map onto one container port.
		if cRange && (!hRange || hHi-hLo != cHi-cLo) {
			return ps, false
		}
	}
	ps.Host = hostPart
	ps.Container = containerPart
	return ps, true
}

// parsePortRange parses "8080" or "8000-8010".
func parsePortRange(s string) (lo, hi int, isRange, ok bool) {
	if i := strings.Index(s, "-"); i >= 0 {
		lo, errLo := strconv.Atoi(s[:i])
		hi, errHi := strconv.Atoi(s[i+1:])
		if errLo != nil || errHi != nil || !validPortNumber(lo) || !validPortNumber(hi) || lo > hi {
			return 0, 0, false, false
		}
		return lo, hi, true, true
	}
	n, err := strconv.Atoi(s)
	if err != nil || !validPortNumber(n) {
		return 0, 0, false, false
	}
	return n, n, false, true
}

func validPortNumber(n int) bool { return n >= 1 && n <= 65535 }
