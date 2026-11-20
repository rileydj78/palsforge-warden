//go:build !windows

package worlds

import "time"

// Linux file systems don't reliably report when a file was created, so first-join times come
// only from what Warden saw there.
func created(string) time.Time { return time.Time{} }
