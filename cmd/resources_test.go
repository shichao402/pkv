package cmd

import (
	"net"
	"strings"
	"testing"
)

func TestBuildAddSSHKeyParamsGeneratesWhenPrivatePathMissing(t *testing.T) {
	withAddSSHFlags(t, func() {
		addNameFlag = "generated-key"
		params, err := buildAddSSHKeyParams("prod")
		if err != nil {
			t.Fatalf("buildAddSSHKeyParams: %v", err)
		}
		if !params.Generated {
			t.Fatal("Generated = false, want true")
		}
		if params.KeyName != "generated-key" {
			t.Fatalf("KeyName = %q, want generated-key", params.KeyName)
		}
		if !strings.Contains(params.OpenSSHKey, "OPENSSH PRIVATE KEY") {
			t.Fatal("OpenSSHKey missing OpenSSH private key")
		}
		if !strings.HasPrefix(params.PublicKey, "ssh-ed25519 ") {
			t.Fatalf("PublicKey = %q, want ssh-ed25519 key", params.PublicKey)
		}
		if len(params.Hosts) != 0 {
			t.Fatalf("Hosts = %v, want empty", params.Hosts)
		}
	})
}

func TestBuildAddSSHKeyParamsRejectsPublicKeyWithoutPrivatePath(t *testing.T) {
	withAddSSHFlags(t, func() {
		addNameFlag = "generated-key"
		addSSHPubFlag = "ssh-ed25519 AAAA..."
		_, err := buildAddSSHKeyParams("prod")
		if err == nil || !strings.Contains(err.Error(), "--pub requires --priv") {
			t.Fatalf("error = %v, want --pub requires --priv", err)
		}
	})
}

func withAddSSHFlags(t *testing.T, fn func()) {
	t.Helper()
	oldPriv := addSSHPrivFlag
	oldPub := addSSHPubFlag
	oldName := addNameFlag
	oldGenerate := addSSHGenerateFlag
	oldType := addSSHTypeFlag
	oldBits := addSSHBitsFlag
	oldComment := addSSHCommentFlag
	oldHosts := append([]string(nil), addSSHHostsFlag...)
	defer func() {
		addSSHPrivFlag = oldPriv
		addSSHPubFlag = oldPub
		addNameFlag = oldName
		addSSHGenerateFlag = oldGenerate
		addSSHTypeFlag = oldType
		addSSHBitsFlag = oldBits
		addSSHCommentFlag = oldComment
		addSSHHostsFlag = oldHosts
	}()
	addSSHPrivFlag = ""
	addSSHPubFlag = ""
	addNameFlag = ""
	addSSHGenerateFlag = false
	addSSHTypeFlag = "ed25519"
	addSSHBitsFlag = 4096
	addSSHCommentFlag = "test@host"
	addSSHHostsFlag = nil
	fn()
}

func TestIsPrivateIPv4(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		// RFC1918
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"172.15.0.1", false},
		{"172.32.0.1", false},
		{"192.168.0.1", true},
		{"192.168.255.255", true},
		{"192.169.0.1", false},
		// RFC6598 CGNAT
		{"100.64.0.1", true},
		{"100.127.255.255", true},
		{"100.63.255.255", false},
		{"100.128.0.1", false},
		// Public
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"203.0.113.5", false},
	}
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := net.ParseIP(tt.ip).To4()
			if ip == nil {
				t.Fatalf("ParseIP(%q) is not IPv4", tt.ip)
			}
			if got := isPrivateIPv4(ip); got != tt.want {
				t.Errorf("isPrivateIPv4(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestIsPrivateIPv4_NonIPv4(t *testing.T) {
	// 16-byte IPv6 representation should not be classified as private.
	ip := net.ParseIP("2001:db8::1")
	if isPrivateIPv4(ip) {
		t.Errorf("isPrivateIPv4(IPv6) = true, want false")
	}
}

func TestLocalIPv4_RunsAndReturnsValidOrEmpty(t *testing.T) {
	// localIPv4 walks live interfaces; we can't mock that cleanly, but we
	// can at least assert it doesn't panic and returns either "" or a
	// parseable IPv4 string.
	got := localIPv4()
	if got == "" {
		return
	}
	if ip := net.ParseIP(got); ip == nil || ip.To4() == nil {
		t.Errorf("localIPv4() = %q, not a valid IPv4 address", got)
	}
}

func mustIPv4(t *testing.T, s string) net.IP {
	t.Helper()
	ip := net.ParseIP(s).To4()
	if ip == nil {
		t.Fatalf("ParseIP(%q) is not IPv4", s)
	}
	return ip
}

func TestSelectIPv4_Empty(t *testing.T) {
	if got := selectIPv4(nil); got != "" {
		t.Errorf("selectIPv4(nil) = %q, want \"\"", got)
	}
	if got := selectIPv4([]ipv4Candidate{}); got != "" {
		t.Errorf("selectIPv4([]) = %q, want \"\"", got)
	}
}

func TestSelectIPv4_PublicWinsOverPrivate(t *testing.T) {
	// Even if the private IP would sort first by interface name, public
	// must still win.
	got := selectIPv4([]ipv4Candidate{
		{iface: "en0", ip: mustIPv4(t, "192.168.1.10")},
		{iface: "eth1", ip: mustIPv4(t, "203.0.113.42")},
	})
	if got != "203.0.113.42" {
		t.Errorf("selectIPv4 = %q, want 203.0.113.42", got)
	}
}

func TestSelectIPv4_PrivateFallback(t *testing.T) {
	got := selectIPv4([]ipv4Candidate{
		{iface: "en0", ip: mustIPv4(t, "10.0.0.5")},
		{iface: "eth1", ip: mustIPv4(t, "192.168.1.10")},
	})
	// All private → sorted by iface name; "en0" < "eth1".
	if got != "10.0.0.5" {
		t.Errorf("selectIPv4 = %q, want 10.0.0.5", got)
	}
}

func TestSelectIPv4_DeterministicAcrossInputOrder(t *testing.T) {
	// Two identical candidate sets in different input order must produce
	// the same result.
	a := []ipv4Candidate{
		{iface: "eth1", ip: mustIPv4(t, "203.0.113.99")},
		{iface: "en0", ip: mustIPv4(t, "203.0.113.42")},
	}
	b := []ipv4Candidate{
		{iface: "en0", ip: mustIPv4(t, "203.0.113.42")},
		{iface: "eth1", ip: mustIPv4(t, "203.0.113.99")},
	}
	got1, got2 := selectIPv4(a), selectIPv4(b)
	if got1 != got2 {
		t.Errorf("selectIPv4 not deterministic: %q vs %q", got1, got2)
	}
	// Both public → iface ascending picks "en0" → 203.0.113.42.
	if got1 != "203.0.113.42" {
		t.Errorf("selectIPv4 = %q, want 203.0.113.42", got1)
	}
}

func TestSelectIPv4_TieBreakOnIPWithinSameInterface(t *testing.T) {
	// Same interface, two public IPs → tiebreak by IP byte order.
	got := selectIPv4([]ipv4Candidate{
		{iface: "eth0", ip: mustIPv4(t, "203.0.113.99")},
		{iface: "eth0", ip: mustIPv4(t, "198.51.100.7")},
	})
	if got != "198.51.100.7" {
		t.Errorf("selectIPv4 = %q, want 198.51.100.7", got)
	}
}

func TestSelectIPv4_DoesNotMutateInput(t *testing.T) {
	in := []ipv4Candidate{
		{iface: "eth1", ip: mustIPv4(t, "203.0.113.99")},
		{iface: "en0", ip: mustIPv4(t, "192.168.1.10")},
	}
	first := in[0]
	_ = selectIPv4(in)
	if in[0].iface != first.iface || !in[0].ip.Equal(first.ip) {
		t.Errorf("selectIPv4 mutated input: in[0] = %+v, want %+v", in[0], first)
	}
}

func TestIsVirtualIface(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		// Real physical / primary interfaces — must NOT be virtual.
		{"eth0", false},
		{"eth1", false},
		{"en0", false},
		{"en1", false},
		{"en4", false},
		{"ens3", false},
		{"ens33", false},
		{"enp0s3", false},
		{"enp3s0f0", false},
		{"wlan0", false},
		{"wlp2s0", false},

		// Container / virtualization.
		{"docker0", true},
		{"docker_gwbridge", true},
		{"br-1234abcd", true},
		{"veth1234", true},
		{"virbr0", true},
		{"vnet0", true},
		{"vmnet1", true},
		{"vmnet8", true},
		{"vboxnet0", true},

		// VPN / overlay.
		{"tailscale0", true},
		{"tun0", true},
		{"tunl0", true},
		{"tap0", true},
		{"utun0", true},
		{"utun4", true},
		{"wg0", true},
		{"zt0", true},

		// k8s CNIs.
		{"cni0", true},
		{"flannel.1", true},
		{"cali1234", true},
		{"weave", true},

		// macOS internals.
		{"awdl0", true},
		{"llw0", true},
		{"gif0", true},
		{"stf0", true},
		{"bridge0", true},
		{"ap1", true},
		{"anpi0", true},

		// Misc.
		{"ppp0", true},

		// Case-insensitivity.
		{"Docker0", true},
		{"UTUN0", true},

		// Loopback isn't filtered here (handled by FlagLoopback in caller),
		// but the name itself should not be mistaken for virtual.
		{"lo", false},
		{"lo0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isVirtualIface(tt.name); got != tt.want {
				t.Errorf("isVirtualIface(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
