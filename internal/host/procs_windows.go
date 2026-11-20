//go:build windows

package host

import (
	"encoding/csv"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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
