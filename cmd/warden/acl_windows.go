package main

import (
	"log"
	"os/exec"
	"strings"
	"syscall"
)

// protectDir lets only SYSTEM and Administrators into one of Warden's folders. Windows ignores
// Go's 0700/0600 modes, and C:\ProgramData lets every user read (and add files) by default; these
// folders hold warden.json's secrets, the SFTP key, the database and the instance list. SIDs are
// used, not names, so it works on any language of Windows.
func protectDir(dir string) {
	run := func(args ...string) {
		cmd := exec.Command("icacls", args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("couldn't restrict %s to SYSTEM and Administrators: %v %s", dir, err, strings.TrimSpace(string(out)))
		}
	}
	// The folder alone gets the rule (inherited by everything in it)...
	run(dir, "/inheritance:r", "/grant:r", "*S-1-5-18:(OI)(CI)F", "*S-1-5-32-544:(OI)(CI)F", "/C", "/Q")
	// ...and what's already inside is reset to inherit it. (Granting folder rules to files with
	// /T, as an earlier build did, leaves files with no access at all.)
	run(dir+`\*`, "/reset", "/T", "/C", "/Q")
}
