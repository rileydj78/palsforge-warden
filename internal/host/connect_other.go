//go:build !windows

package host

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Connection on Linux: the UDP port from /proc/net/udp(6) and the server's command line; the
// firewall (ufw, firewalld, nftables) isn't read.
func (h *Host) Connection(port int) ConnFacts {
	f := ConnFacts{LANAddrs: lanAddrs()}
	hexPort := fmt.Sprintf(":%04X", port)
	for _, p := range []string{"/proc/net/udp", "/proc/net/udp6"} {
		if b, err := os.ReadFile(p); err == nil {
			for _, line := range strings.Split(string(b), "\n")[1:] {
				fs := strings.Fields(line)
				if len(fs) > 2 && strings.HasSuffix(fs[1], hexPort) {
					f.Listening = true
				}
			}
		}
	}
	if pids, err := pidsOf(h.cfg.Abs(h.cfg.Control.Exe)); err == nil && len(pids) > 0 {
		if b, err := os.ReadFile(filepath.Join("/proc", fmt.Sprint(pids[0]), "cmdline")); err == nil {
			f.CommandLine = string(bytes.ReplaceAll(bytes.TrimRight(b, "\x00"), []byte{0}, []byte{' '}))
		}
	}
	return f
}
