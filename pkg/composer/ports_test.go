package composer

import "testing"

func TestParsePortSpecValid(t *testing.T) {
	cases := []struct {
		in   string
		want portSpec
	}{
		{"80", portSpec{Container: "80"}},
		{"8080:80", portSpec{Host: "8080", Container: "80"}},
		{"127.0.0.1:8080:80", portSpec{HostIP: "127.0.0.1", Host: "8080", Container: "80"}},
		{"127.0.0.1::80", portSpec{HostIP: "127.0.0.1", Container: "80"}},
		{"8080:80/udp", portSpec{Host: "8080", Container: "80", Proto: "/udp"}},
		{"8080/sctp", portSpec{Container: "8080", Proto: "/sctp"}},
		{"[::1]:8080:80", portSpec{HostIP: "::1", Host: "8080", Container: "80"}},
		{"[::1]::80", portSpec{HostIP: "::1", Container: "80"}},
		{"[2001:db8::1]:8080:80/tcp", portSpec{HostIP: "2001:db8::1", Host: "8080", Container: "80", Proto: "/tcp"}},
		{"8000-8010:8000-8010", portSpec{Host: "8000-8010", Container: "8000-8010"}},
		{"8000-8010:80", portSpec{Host: "8000-8010", Container: "80"}},
		{"127.0.0.1:8000-8010:8000-8010/udp", portSpec{HostIP: "127.0.0.1", Host: "8000-8010", Container: "8000-8010", Proto: "/udp"}},
		{"127.0.0.1::8000-8010", portSpec{HostIP: "127.0.0.1", Container: "8000-8010"}},
		{"80-90", portSpec{Container: "80-90"}},
	}
	for _, tc := range cases {
		got, ok := parsePortSpec(tc.in)
		if !ok || got != tc.want {
			t.Errorf("parsePortSpec(%q) = %+v, %v; want %+v, true", tc.in, got, ok, tc.want)
		}
	}
}

func TestParsePortSpecInvalid(t *testing.T) {
	for _, in := range []string{
		"", " ", "abc", "0", "99999", "1:2:3:4", ":80",
		"::1:8080:80",       // IPv6 without brackets
		"[::1]:80",          // IPv6 needs host:container or :container
		"[::1]8080:80",      // missing separator after the bracket
		"[::1:8080:80",      // unterminated bracket
		"[127.0.0.1]:80:80", // IPv4 doesn't go in brackets
		"8080/icmp",
		"8010-8000:80",        // reversed range
		"0-10",                // port 0
		"70000-70001:80",      // out of range
		"8000-8010:8000-8020", // length mismatch
		"80:8000-8010",        // container range needs a host range
		"8000-:80",
	} {
		if got, ok := parsePortSpec(in); ok {
			t.Errorf("parsePortSpec(%q) = %+v, want invalid", in, got)
		}
	}
}

func TestPortExposureIPv6AndRanges(t *testing.T) {
	cases := []struct {
		port    string
		exposed bool
	}{
		{"[::1]:8080:80", false},
		{"[::]:8080:80", true},
		{"[2001:db8::1]:8080:80", true},
		{"8000-8010:80", true},
		{"127.0.0.1:8000-8010:8000-8010", false},
		{"0.0.0.0:8000-8010:80", true},
	}
	for _, tc := range cases {
		if got := isPortExposedToAllInterfaces(tc.port); got != tc.exposed {
			t.Errorf("isPortExposedToAllInterfaces(%q) = %v, want %v", tc.port, got, tc.exposed)
		}
	}
}

func TestSuggestLocalhostPortIPv6AndRanges(t *testing.T) {
	cases := []struct{ in, want string }{
		{"8000-8010:80", "127.0.0.1:8000-8010:80"},
		{"8000-8010:8000-8010/udp", "127.0.0.1:8000-8010:8000-8010/udp"},
		{"80-90", "127.0.0.1::80-90"},
		{"[::]:8080:80", "[::1]:8080:80"},
		{"[::]::80/udp", "[::1]::80/udp"},
		{"8080/sctp", "127.0.0.1::8080/sctp"},
	}
	for _, tc := range cases {
		if got := suggestLocalhostPort(tc.in); got != tc.want {
			t.Errorf("suggestLocalhostPort(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if !isValidPort(tc.want) {
			t.Errorf("suggestion %q is not itself a valid port entry", tc.want)
		}
	}
}

func TestValidateWarnsOnIPv6AllInterfacesPort(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("web")
	svc.Image = "nginx:1.27"
	svc.Ports = []string{"[::]:8080:80"}
	r := c.Validate()
	if hasError(r, "ports") {
		t.Fatalf("valid IPv6 port reported as an error: %+v", r.Errors)
	}
	if !hasWarning(r, "ports") {
		t.Fatalf("expected an all-interfaces warning, got %+v", r.Warnings)
	}
	svc.Ports = []string{"[::1]:8080:80", "127.0.0.1:9000-9010:9000-9010"}
	r = c.Validate()
	if hasError(r, "ports") || hasWarning(r, "ports") {
		t.Fatalf("loopback-bound IPv6 / range ports must be clean, got %+v", r)
	}
}

func TestLongSyntaxIPv6AndRangePorts(t *testing.T) {
	cfg := importString(t, `
services:
  web:
    image: nginx:1.27
    ports:
      - target: 80
        published: "8000-8010"
        host_ip: "::"
`)
	if !hasWarning(cfg.Validate(), "ports") {
		t.Fatal("long-syntax IPv6 wildcard port with a range must warn")
	}
	safe := importString(t, `
services:
  web:
    image: nginx:1.27
    ports:
      - target: 80
        published: 8080
        host_ip: "::1"
`)
	if hasWarning(safe.Validate(), "ports") {
		t.Fatal("long-syntax IPv6 loopback port must not warn")
	}
}
