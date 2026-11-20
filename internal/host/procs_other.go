//go:build !windows

package host

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// pidsOf lists the process ids whose program is exe (matched by file name in /proc).
func pidsOf(exe string) ([]int, error) {
	name := []byte(filepath.Base(exe))
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil || len(cmdline) == 0 {
			continue
		}
		argv0 := cmdline
		if i := bytes.IndexByte(cmdline, 0); i >= 0 {
			argv0 = cmdline[:i]
		}
		if bytes.Equal([]byte(filepath.Base(string(argv0))), name) {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

func kill(pid int) error { return syscall.Kill(pid, syscall.SIGKILL) }

func setAffinity(pid int, mask uint64) error {
	return errors.New("CPU affinity is only set on Windows; use taskset on Linux")
}

func hideWindow(cmd *exec.Cmd) {}

// memOf returns the resident memory of the processes running exe, in bytes.
func memOf(exe string) (int64, error) {
	pids, err := pidsOf(exe)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, pid := range pids {
		b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
		if err != nil {
			continue
		}
		for _, line := range bytes.Split(b, []byte("\n")) {
			if f := bytes.Fields(line); len(f) >= 2 && string(f[0]) == "VmRSS:" {
				if kb, err := strconv.ParseInt(string(f[1]), 10, 64); err == nil {
					total += kb * 1024
				}
			}
		}
	}
	return total, nil
}

// openPorts lists the TCP ports listening on an address other than this machine's loopback.
func openPorts() map[int]bool {
	open := map[int]bool{}
	for _, f := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, line := range bytes.Split(b, []byte("\n"))[1:] {
			fs := bytes.Fields(line)
			if len(fs) < 4 || string(fs[3]) != "0A" { // 0A = LISTEN
				continue
			}
			local := string(fs[1])
			i := strings.LastIndexByte(local, ':')
			if i < 0 {
				continue
			}
			hexIP := local[:i]
			loop := hexIP == "0100007F" || hexIP == "00000000000000000000000001000000" // 127.0.0.1, ::1
			if strings.HasSuffix(hexIP, "7F") && len(hexIP) == 8 {
				loop = true // 127.x.x.x (little-endian)
			}
			if loop {
				continue
			}
			if p, err := strconv.ParseInt(local[i+1:], 16, 32); err == nil {
				open[int(p)] = true
			}
		}
	}
	return open
}

// machineMem returns the machine's used and total memory in bytes.
func machineMem() (used, total uint64) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	var tot, avail uint64
	for _, line := range bytes.Split(b, []byte("\n")) {
		f := bytes.Fields(line)
		if len(f) < 2 {
			continue
		}
		n, _ := strconv.ParseUint(string(f[1]), 10, 64)
		switch string(f[0]) {
		case "MemTotal:":
			tot = n * 1024
		case "MemAvailable:":
			avail = n * 1024
		}
	}
	return tot - avail, tot
}

// cpuTimes returns the machine's idle and total CPU time so far (jiffies).
func cpuTimes() (idle, total uint64) {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0
	}
	f := bytes.Fields(bytes.SplitN(b, []byte("\n"), 2)[0])
	for i, x := range f[1:] {
		n, _ := strconv.ParseUint(string(x), 10, 64)
		total += n
		if i == 3 || i == 4 { // idle, iowait
			idle += n
		}
	}
	return idle, total
}

// diskFree returns free and total bytes on the filesystem holding path.
func diskFree(path string) (free, total uint64) {
	var st syscall.Statfs_t
	if syscall.Statfs(path, &st) != nil {
		return 0, 0
	}
	return st.Bavail * uint64(st.Bsize), st.Blocks * uint64(st.Bsize)
}

// netBytes returns bytes received and sent on all interfaces except loopback so far.
func netBytes() (rx, tx uint64) {
	b, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		name, rest, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(name) == "lo" {
			continue
		}
		f := strings.Fields(rest)
		if len(f) < 9 {
			continue
		}
		r, _ := strconv.ParseUint(f[0], 10, 64)
		t, _ := strconv.ParseUint(f[8], 10, 64)
		rx, tx = rx+r, tx+t
	}
	return rx, tx
}
