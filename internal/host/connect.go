package host

import (
	"net"
	"strings"
)

// ConnFacts is what this machine can tell about players reaching the game (no outside calls).
type ConnFacts struct {
	Listening   bool     `json:"listening"`   // the game's UDP port is open on this machine
	FirewallOn  bool     `json:"firewallOn"`  // Windows Firewall is on for some profile
	Allowed     bool     `json:"allowed"`     // an enabled inbound allow rule covers the port or the program
	Known       bool     `json:"known"`       // the firewall could be read
	CommandLine string   `json:"commandLine"` // the running server's command line ("" if unknown)
	LANAddrs    []string `json:"lanAddrs"`    // this machine's private IPv4 addresses (for port forwarding)
}

func lanAddrs() []string {
	var out []string
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 || strings.Contains(strings.ToLower(i.Name), "tailscale") {
			continue
		}
		as, _ := i.Addrs()
		for _, a := range as {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil && ipn.IP.IsPrivate() {
				out = append(out, ipn.IP.String())
			}
		}
	}
	return out
}
