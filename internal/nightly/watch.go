package nightly

import (
	"context"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/notify"
)

// WatchDown tells the admins when the server stops answering for 3 minutes and when it is back,
// except during the routine or a stop an admin asked for.
func (r *Runner) WatchDown(ctx context.Context) {
	if r.Notify == nil || !r.Notify.Enabled() {
		return
	}
	misses, down := 0, false
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}
		busy, _ := r.Busy()
		planned := busy || !r.Host.State().Wanted
		if r.Game.Alive(ctx) {
			if down {
				r.Notify.Send(ctx, notify.Info, "", "The server is back", "It is answering again.")
			}
			misses, down = 0, false
			continue
		}
		if planned {
			misses = 0
			continue
		}
		misses++
		if misses == 3 && !down {
			down = true
			r.Notify.Send(ctx, notify.Urgent, "down", "The server isn't answering", "No answer for 3 minutes. Check the Server page; Warden restarts it if it runs the server itself.")
		}
	}
}
