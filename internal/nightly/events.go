package nightly

import (
	"context"
	"fmt"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/config"
	"github.com/rileydj78/palsforge-warden/internal/events"
	"github.com/rileydj78/palsforge-warden/internal/gamesettings"
)

// RunEvents switches weekly events on and off. Each switch restarts the server with 5- and
// 1-minute warnings that announce the event; the normal values come back when it ends.
func (r *Runner) RunEvents(ctx context.Context, evs []events.Event) {
	if len(evs) == 0 {
		return
	}
	data := r.Backups.DataDir
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}
		if busy, _ := r.Busy(); busy || !r.Host.State().Wanted {
			continue
		}
		st := events.Load(data)
		want := events.Want(evs, time.Now())
		switch {
		case want != nil && st.Active != want.Name:
			if st.Active != "" { // one event hands over to another: the normal values first
				gamesettings.AddPending(data, st.Normal)
			}
			normal := map[string]string{}
			if f, err := gamesettings.Read(config.SettingsPath(r.Cfg.ServerDir)); err == nil {
				for k := range want.Settings {
					normal[k] = f.Values[k]
				}
			}
			if st.Active != "" {
				for k, v := range st.Normal {
					if _, ok := normal[k]; ok {
						normal[k] = v
					}
				}
			}
			gamesettings.AddPending(data, want.Settings)
			events.Save(data, events.State{Active: want.Name, Normal: normal})
			r.Store.AddEvent(time.Now(), "event", want.Name+" starts")
			r.Game.Announce(ctx, fmt.Sprintf("%s starts in 5 minutes! The server restarts to switch it on.", want.Name))
			r.Restart(ctx, "event: "+want.Name, []int{5, 1})
		case want == nil && st.Active != "":
			gamesettings.AddPending(data, st.Normal)
			events.Save(data, events.State{})
			r.Store.AddEvent(time.Now(), "event", st.Active+" ends")
			r.Game.Announce(ctx, fmt.Sprintf("%s ends in 5 minutes. Thanks for playing!", st.Active))
			r.Restart(ctx, "event over: "+st.Active, []int{5, 1})
		}
	}
}
