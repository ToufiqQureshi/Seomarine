package geo

import "testing"

func TestCountry(t *testing.T) {
	lookup, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, ip, want string
	}{
		{"public IPv4", "8.8.8.8", "US"},
		{"public IPv6", "2001:4860:4860::8888", "CA"},
		{"private IPv4", "192.168.1.1", ""},
		{"private IPv6", "fd00::1", ""},
		{"invalid header", "not-an-ip", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := lookup.Country(tt.ip)
			if err != nil || got != tt.want {
				t.Fatalf("Country(%q) = %q, %v; want %q", tt.ip, got, err, tt.want)
			}
		})
	}
}
