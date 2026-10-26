package easytier

import (
	"strings"
	"testing"
)

func TestSuggestDeviceIP(t *testing.T) {
	for _, test := range []struct {
		name string
		cidr string
		used []string
		want string
	}{
		{"empty network picks first host", "10.14.14.0/24", nil, "10.14.14.1/24"},
		{"skips occupied hosts", "10.14.14.0/24", []string{"10.14.14.1/24", "10.14.14.2/24"}, "10.14.14.3/24"},
		{"skips network and broadcast", "10.14.14.0/30", nil, "10.14.14.1/30"},
		{"used entries without prefix still count", "10.14.14.0/24", []string{"10.14.14.1"}, "10.14.14.2/24"},
		{"unparseable used entries are ignored", "10.14.14.0/24", []string{"junk", "10.14.14.1/24"}, "10.14.14.2/24"},
		{"slash 31 has no broadcast", "10.14.14.0/31", nil, "10.14.14.0/31"},
		{"slash 32 single host", "10.14.14.7/32", nil, "10.14.14.7/32"},
		{"unmasked cidr is normalized", "10.14.14.9/24", []string{"10.14.14.1/24"}, "10.14.14.2/24"},
	} {
		got, err := SuggestDeviceIP(test.cidr, test.used)
		if err != nil || got != test.want {
			t.Errorf("%s: SuggestDeviceIP(%q) = %q, %v; want %q", test.name, test.cidr, got, err, test.want)
		}
	}
	for _, cidr := range []string{"", "10.14.14.0", "fd00::/64", "10.14.14.0/33"} {
		if got, err := SuggestDeviceIP(cidr, nil); err == nil {
			t.Errorf("SuggestDeviceIP(%q) = %q, want error", cidr, got)
		}
	}
	// A full /30 has no host left once both usable addresses are taken.
	if _, err := SuggestDeviceIP("10.14.14.0/30", []string{"10.14.14.1/30", "10.14.14.2/30"}); err == nil {
		t.Error("exhausted network should error")
	}
}

func TestSuggestPortalEndpoints(t *testing.T) {
	got := SuggestPortalEndpoints([]string{
		"tcp://0.0.0.0:11010",
		"udp://[::]:11010",
		"tcp://127.0.0.1:11010",
		"wg://1.2.3.4:11010",
		"tcp://vpn.example.com:11010",
		"udp://1.2.3.4:11011", // duplicate host collapses
	}, "0.0.0.0:51820")
	want := []string{"1.2.3.4:51820", "vpn.example.com:51820"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := SuggestPortalEndpoints([]string{"tcp://0.0.0.0:11010"}, "0.0.0.0:51820"); len(got) != 0 {
		t.Fatalf("wildcard-only listeners suggest nothing: %v", got)
	}
	if got := SuggestPortalEndpoints([]string{"tcp://1.2.3.4:11010"}, "not-a-listen"); got != nil {
		t.Fatalf("bad portal listen suggests nothing: %v", got)
	}
	// IPv6 hosts are bracketed for WireGuard.
	got = SuggestPortalEndpoints([]string{"tcp://[2001:db8::1]:11010"}, "0.0.0.0:51820")
	if len(got) != 1 || got[0] != "[2001:db8::1]:51820" {
		t.Fatalf("IPv6 suggestion: %v", got)
	}
}
