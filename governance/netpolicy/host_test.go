package netpolicy

import (
	"net"
	"testing"
)

func TestParseHostTokenLiterals(t *testing.T) {
	cases := []struct {
		token string
		want  string
	}{
		// dotted quad — loopback
		{"127.0.0.1", "127.0.0.1"},
		{"127.0.0.2", "127.0.0.2"},
		{"127.255.255.255", "127.255.255.255"},
		// dotted quad — private
		{"10.0.0.1", "10.0.0.1"},
		{"10.255.255.255", "10.255.255.255"},
		{"172.16.0.1", "172.16.0.1"},
		{"172.31.255.255", "172.31.255.255"},
		{"192.168.0.1", "192.168.0.1"},
		{"192.168.255.255", "192.168.255.255"},
		// dotted quad — link-local / metadata
		{"169.254.169.254", "169.254.169.254"},
		{"169.254.0.1", "169.254.0.1"},
		// dotted quad — reserved
		{"100.64.0.1", "100.64.0.1"},
		{"192.0.0.1", "192.0.0.1"},
		{"192.0.2.1", "192.0.2.1"},
		{"198.18.0.1", "198.18.0.1"},
		{"198.51.100.1", "198.51.100.1"},
		{"203.0.113.1", "203.0.113.1"},
		{"224.0.0.1", "224.0.0.1"},
		{"239.255.255.255", "239.255.255.255"},
		{"240.0.0.1", "240.0.0.1"},
		{"255.255.255.255", "255.255.255.255"},
		// dotted quad — unspecified / public
		{"0.0.0.0", "0.0.0.0"},
		{"0.1.2.3", "0.1.2.3"},
		{"8.8.8.8", "8.8.8.8"},
		{"1.1.1.1", "1.1.1.1"},
		{"93.184.216.34", "93.184.216.34"},
		// inet_aton forms
		{"2130706433", "127.0.0.1"},
		{"0x7f000001", "127.0.0.1"},
		{"0177.0.0.1", "127.0.0.1"},
		{"127.1", "127.0.0.1"},
		{"0x7f.1", "127.0.0.1"},
		{"0177.1", "127.0.0.1"},
		{"127.0.1", "127.0.0.1"},
		{"3232235777", "192.168.1.1"},  // 192.168.1.1
		{"0xC0A80101", "192.168.1.1"},  // 192.168.1.1
		{"192.168.257", "192.168.1.1"}, // last part is 16 bits
		// IPv6
		{"::1", "::1"},
		{"::", "::"},
		{"::ffff:127.0.0.1", "127.0.0.1"},
		{"::ffff:10.0.0.1", "10.0.0.1"},
		{"fe80::1", "fe80::1"},
		{"fe80::1%eth0", "fe80::1"},
		{"fc00::1", "fc00::1"},
		{"fdff::1", "fdff::1"},
		{"2001:db8::1", "2001:db8::1"},
		{"ff02::1", "ff02::1"},
		{"2001:4860:4860::8888", "2001:4860:4860::8888"},
		// bracketed
		{"[::1]", "::1"},
		{"[fe80::1]", "fe80::1"},
		{"[::ffff:127.0.0.1]", "127.0.0.1"},
	}
	if len(cases) < 40 {
		t.Fatalf("literal table must be thorough, has %d cases", len(cases))
	}
	for _, tc := range cases {
		ip, ok := ParseHostToken(tc.token)
		if !ok {
			t.Errorf("ParseHostToken(%q) = not a literal, want %s", tc.token, tc.want)
			continue
		}
		if ip.String() != tc.want {
			t.Errorf("ParseHostToken(%q) = %s, want %s", tc.token, ip.String(), tc.want)
		}
	}
}

func TestParseHostTokenNotLiterals(t *testing.T) {
	names := []string{
		"example.com",
		"localhost",
		"internal.host",
		"",
		"-",
		"10.0.0.0/8",
		"192.168.0.0/16",
		"[::1]:443",    // port is not part of a host token
		"127.0.0.1:80", // port
		"example.com:443",
		"08.1.1.1",  // invalid octal
		"09.1.1.1",  // invalid octal
		"1.2.3.4.5", // too many parts
		"256.1.1.1", // overflow
		"1.2.3.256", // overflow
		"0x",        // empty hex
		"::g",       // bad hex
		"fe80::1%",  // empty zone
		"fe80::1%et h0",
		"99999999999999999999",
		"0x100000000", // overflow past 32 bits
	}
	for _, name := range names {
		if ip, ok := ParseHostToken(name); ok {
			t.Errorf("ParseHostToken(%q) = %v, want not-a-literal", name, ip)
		}
	}
}

func TestClassifyIP(t *testing.T) {
	cases := []struct {
		ip    string
		class HostClass
	}{
		{"127.0.0.1", ClassLoopback},
		{"127.255.255.255", ClassLoopback},
		{"::1", ClassLoopback},
		{"169.254.169.254", ClassLinkLocal},
		{"169.254.0.1", ClassLinkLocal},
		{"fe80::1", ClassLinkLocal},
		{"ff02::1", ClassLinkLocal}, // IPv6 link-local multicast
		{"ff05::1", ClassReserved},  // site-local multicast is reserved
		{"10.0.0.1", ClassPrivate},
		{"10.255.255.255", ClassPrivate},
		{"172.16.0.1", ClassPrivate},
		{"172.31.255.255", ClassPrivate},
		{"172.32.0.1", ClassPublic},
		{"192.168.0.1", ClassPrivate},
		{"192.168.255.255", ClassPrivate},
		{"fc00::1", ClassPrivate},
		{"fdff::1", ClassPrivate},
		{"0.0.0.0", ClassUnspecified},
		{"0.1.2.3", ClassUnspecified},
		{"::", ClassUnspecified},
		{"100.64.0.1", ClassReserved},
		{"100.127.255.255", ClassReserved},
		{"192.0.0.1", ClassReserved},
		{"192.0.2.1", ClassReserved},
		{"198.18.0.1", ClassReserved},
		{"198.51.100.1", ClassReserved},
		{"203.0.113.1", ClassReserved},
		{"224.0.0.1", ClassLinkLocal}, // IPv4 link-local multicast
		{"239.255.255.255", ClassReserved},
		{"240.0.0.1", ClassReserved},
		{"255.255.255.255", ClassReserved},
		{"2001:db8::1", ClassReserved},
		{"ff00::1", ClassReserved},
		{"8.8.8.8", ClassPublic},
		{"1.1.1.1", ClassPublic},
		{"93.184.216.34", ClassPublic},
		{"172.15.0.1", ClassPublic},
		{"192.167.255.255", ClassPublic},
		{"2001:4860:4860::8888", ClassPublic},
	}
	for _, tc := range cases {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("test setup: invalid IP %q", tc.ip)
		}
		if got := ClassifyIP(ip); got != tc.class {
			t.Errorf("ClassifyIP(%q) = %s, want %s", tc.ip, got, tc.class)
		}
	}
}

func TestClassifyToken(t *testing.T) {
	literals := map[string]HostClass{
		"127.0.0.1":            ClassLoopback,
		"2130706433":           ClassLoopback,
		"0x7f000001":           ClassLoopback,
		"0177.0.0.1":           ClassLoopback,
		"127.1":                ClassLoopback,
		"169.254.169.254":      ClassLinkLocal,
		"fe80::1":              ClassLinkLocal,
		"10.0.0.1":             ClassPrivate,
		"fc00::1":              ClassPrivate,
		"0.0.0.0":              ClassUnspecified,
		"::":                   ClassUnspecified,
		"100.64.0.1":           ClassReserved,
		"192.0.2.1":            ClassReserved,
		"8.8.8.8":              ClassPublic,
		"2001:4860:4860::8888": ClassPublic,
	}
	for token, want := range literals {
		class, ok := ClassifyToken(token)
		if !ok {
			t.Errorf("ClassifyToken(%q) = not a literal, want %s", token, want)
			continue
		}
		if class != want {
			t.Errorf("ClassifyToken(%q) = %s, want %s", token, class, want)
		}
	}
	for _, name := range []string{"example.com", "localhost", "08.1.1.1", "127.0.0.1:80"} {
		if class, ok := ClassifyToken(name); ok {
			t.Errorf("ClassifyToken(%q) = (%s, true), want not-a-literal", name, class)
		}
	}
}

// TestClassifyIPv4MatchesClassifyIP locks the allocation-free integer
// classifier and the CIDR/predicate classifier to the same answer.
func TestClassifyIPv4MatchesClassifyIP(t *testing.T) {
	ips := []string{
		"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.0.1", "169.254.169.254",
		"100.64.0.1", "192.0.2.1", "198.18.0.1", "198.51.100.1", "203.0.113.1",
		"224.0.0.1", "239.255.255.255", "240.0.0.1", "255.255.255.255",
		"0.0.0.0", "0.1.2.3", "8.8.8.8", "1.1.1.1", "172.15.0.1", "172.32.0.1",
		"192.167.255.255", "100.63.255.255", "198.17.255.255", "223.255.255.255",
	}
	for _, s := range ips {
		ip := net.ParseIP(s)
		v4 := ip.To4()
		if v4 == nil {
			t.Fatalf("test setup: %q is not IPv4", s)
		}
		v := uint32(v4[0])<<24 | uint32(v4[1])<<16 | uint32(v4[2])<<8 | uint32(v4[3])
		want := ClassifyIP(ip)
		if got := classifyIPv4(v); got != want {
			t.Errorf("classifyIPv4(%s) = %s, ClassifyIP = %s", s, got, want)
		}
	}
}

func BenchmarkClassifyIPLiteral(b *testing.B) {
	ip := net.ParseIP("8.8.8.8")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ClassifyIP(ip)
	}
}

func BenchmarkClassifyTokenLiteral(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = ClassifyToken("192.168.1.1")
	}
}

func BenchmarkClassifyTokenHostname(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = ClassifyToken("example.com")
	}
}
