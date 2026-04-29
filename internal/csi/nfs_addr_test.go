package csi

import "testing"

func TestValidatePrivateNFSEndpoints(t *testing.T) {
	cases := []struct {
		name    string
		server  string
		clients string
		wantErr bool
	}{
		{"empty (nfs disabled)", "", "", false},
		{"rfc1918 ipv4 server", "192.168.10.204", "10.42.0.0/16", false},
		{"loopback server", "127.0.0.1", "127.0.0.0/8", false},
		{"hostname is opaque, allowed", "novanas.local", "10.0.0.0/8", false},
		{"wildcard client", "10.0.0.5", "*", false},
		{"multiple private CIDRs", "10.0.0.5", "10.42.0.0/16, 10.43.0.0/16", false},
		{"ipv6 ULA", "fd00::1", "fd00::/8", false},
		{"public ipv4 server", "8.8.8.8", "10.0.0.0/8", true},
		{"public ipv4 in client list", "10.0.0.5", "10.42.0.0/16,8.8.8.8/32", true},
		{"public CIDR", "10.0.0.5", "0.0.0.0/0", true},
		{"server with port, public", "8.8.8.8:2049", "10.0.0.0/8", true},
		{"server with port, private", "10.0.0.5:2049", "10.0.0.0/8", false},
		{"malformed CIDR", "10.0.0.5", "not-a-cidr/abc", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validatePrivateNFSEndpoints(c.server, c.clients)
			if (err != nil) != c.wantErr {
				t.Fatalf("validate(%q,%q): err=%v wantErr=%v", c.server, c.clients, err, c.wantErr)
			}
		})
	}
}
