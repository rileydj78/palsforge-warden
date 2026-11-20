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
	// Match the full path, not just the program name: with several servers on one machine,
	// each Warden must see (and stop) only its own.
	want := filepath.Clean(exe)
	if r, err := filepath.EvalSymlinks(want); err == nil { // a junction, subst drive or link
		want = r
	}
	snap, _, err := procSnapshot.Call(th32csSnapProcess, 0)
	if snap == uintptr(syscall.InvalidHandle) {
		return nil, fmt.Errorf("process list: %v", err)
	}
	defer syscall.CloseHandle(syscall.Handle(snap))
	var e processEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var pids []int
	name := strings.ToLower(filepath.Base(want))
	for r, _, _ := procFirst.Call(snap, uintptr(unsafe.Pointer(&e))); r != 0; r, _, _ = procNext.Call(snap, uintptr(unsafe.Pointer(&e))) {
		if strings.ToLower(syscall.UTF16ToString(e.ExeFile[:])) != name {
			continue
		}
		p := imagePath(e.ProcessID)
		if r, err := filepath.EvalSymlinks(p); err == nil && p != "" {
			p = r
		}
		if p != "" && strings.EqualFold(filepath.Clean(p), want) {
			pids = append(pids, int(e.ProcessID))
		}
	}
	return pids, nil
}

const th32csSnapProcess = 0x2

type processEntry32 struct {
	Size            uint32
	Usage           uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	Threads         uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [260]uint16
}

var (
	procSnapshot = syscall.NewLazyDLL("kernel32.dll").NewProc("CreateToolhelp32Snapshot")
	procFirst    = syscall.NewLazyDLL("kernel32.dll").NewProc("Process32FirstW")
	procNext     = syscall.NewLazyDLL("kernel32.dll").NewProc("Process32NextW")
	procImage    = syscall.NewLazyDLL("kernel32.dll").NewProc("QueryFullProcessImageNameW")
)

// imagePath is a process's full program path ("" if it can't be read).
func imagePath(pid uint32) string {
	const queryLimited = 0x1000
	h, err := syscall.OpenProcess(queryLimited, false, pid)
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(h)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	if r, _, _ := procImage.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))); r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
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

// netBytes returns bytes received and sent on all interfaces so far ("netstat -e": the first
// line with two numbers is the byte count, whatever the language).
func netBytes() (rx, tx uint64) {
	cmd := exec.Command("netstat", "-e")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		a, e1 := strconv.ParseUint(f[len(f)-2], 10, 64)
		b, e2 := strconv.ParseUint(f[len(f)-1], 10, 64)
		if e1 == nil && e2 == nil {
			return a, b
		}
	}
	return 0, 0
}
