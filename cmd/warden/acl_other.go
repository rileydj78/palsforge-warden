//go:build !windows

package main

import "os"

// protectDir: on Linux and macOS the folders are created 0700; make sure of it.
func protectDir(dir string) { os.Chmod(dir, 0o700) }
