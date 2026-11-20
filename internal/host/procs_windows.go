//go:build windows

package host

import (
	"encoding/csv"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// pidsOf lists the process ids running the program exe (matched by file name).
func pidsOf(exe string) ([]int, error) {
	name := filepath.Base(exe)
	cmd := exec.Command("tasklist", "/FO", "CSV", "/NH", "/FI", "IMAGENAME eq "+name)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tasklist: %w", err)
	}
	rows, _ := csv.NewReader(strings.NewReader(string(out))).ReadAll()
	var pids []int
	for _, r := range rows {
		if len(r) > 1 && strings.EqualFold(r[0], name) {
			if pid, err := strconv.Atoi(r[1]); err == nil {
				pids = append(pids, pid)
			}
		}
	}
	return pids, nil
}

func kill(pid int) error {
	cmd := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("taskkill: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procSetAffinity    = kernel32.NewProc("SetProcessAffinityMask")
	procOpenProcess    = kernel32.NewProc("OpenProcess")
	processSetInfo     = 0x0200
	processQueryLimInf = 0x1000
)

// setAffinity limits a process to the CPU cores in mask.
func setAffinity(pid int, mask uint64) error {
	h, _, err := procOpenProcess.Call(uintptr(processSetInfo|processQueryLimInf), 0, uintptr(pid))
	if h == 0 {
		return fmt.Errorf("open process %d: %v", pid, err)
	}
	defer syscall.CloseHandle(syscall.Handle(h))
	if r, _, err := procSetAffinity.Call(h, uintptr(mask)); r == 0 {
		return fmt.Errorf("set affinity: %v", err)
	}
	return nil
}

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000200} // CREATE_NEW_PROCESS_GROUP
}

// memOf returns the working-set memory of the processes running exe, in bytes.
func memOf(exe string) (int64, error) {
	name := filepath.Base(exe)
	cmd := exec.Command("tasklist", "/FO", "CSV", "/NH", "/FI", "IMAGENAME eq "+name)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	rows, _ := csv.NewReader(strings.NewReader(string(out))).ReadAll()
	var total int64
	for _, r := range rows {
		if len(r) > 4 && strings.EqualFold(r[0], name) {
			kb := strings.Map(func(c rune) rune {
				if c >= '0' && c <= '9' {
					return c
				}
				return -1
			}, r[4]) // "1,234,567 K"
			if n, err := strconv.ParseInt(kb, 10, 64); err == nil {
				total += n * 1024
			}
		}
	}
	return total, nil
}

// openPorts lists the TCP ports listening on an address other than this machine's own loopback.
// Listening sockets are told apart by their foreign address (0.0.0.0:0 / [::]:0), not by the
// localized "LISTENING" word.
func openPorts() map[int]bool {
	open := map[int]bool{}
	for _, proto := range []string{"TCP", "TCPv6"} {
		cmd := exec.Command("netstat", "-ano", "-p", proto)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			f := strings.Fields(line)
			if len(f) < 4 || (f[2] != "0.0.0.0:0" && f[2] != "[::]:0") {
				continue
			}
			addr := f[1]
			i := strings.LastIndex(addr, ":")
			if i < 0 {
				continue
			}
			host := strings.Trim(addr[:i], "[]")
			if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
				continue
			}
			if p, err := strconv.Atoi(addr[i+1:]); err == nil {
				open[p] = true
			}
		}
	}
	return open
}

var (
	procMemStatus   = kernel32.NewProc("GlobalMemoryStatusEx")
	procSystemTimes = kernel32.NewProc("GetSystemTimes")
)

type memStatusEx struct {
	Length, MemoryLoad                                 uint32
	TotalPhys, AvailPhys, TotalPageFile, AvailPageFile uint64
	TotalVirtual, AvailVirtual, AvailExtendedVirtual   uint64
}

// machineMem returns the machine's used and total memory in bytes.
func machineMem() (used, total uint64) {
	var m memStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	if r, _, _ := procMemStatus.Call(uintptr(unsafe.Pointer(&m))); r == 0 {
		return 0, 0
	}
	return m.TotalPhys - m.AvailPhys, m.TotalPhys
}

// cpuTimes returns the machine's idle and total CPU time so far (100 ns units).
func cpuTimes() (idle, total uint64) {
	var i, k, u syscall.Filetime
	if r, _, _ := procSystemTimes.Call(uintptr(unsafe.Pointer(&i)), uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&u))); r == 0 {
		return 0, 0
	}
	ft := func(f syscall.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }
	return ft(i), ft(k) + ft(u) // kernel time includes idle time
}

var procDiskFree = kernel32.NewProc("GetDiskFreeSpaceExW")

// diskFree returns free and total bytes on the drive holding path.
func diskFree(path string) (free, total uint64) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0
	}
	var avail, tot, all uint64
	if r, _, _ := procDiskFree.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&avail)), uintptr(unsafe.Pointer(&tot)), uintptr(unsafe.Pointer(&all))); r == 0 {
		return 0, 0
	}
	return avail, tot
}
