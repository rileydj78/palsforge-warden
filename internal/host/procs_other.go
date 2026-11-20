//go:build !windows

package host

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
