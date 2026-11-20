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
		if h := r.Host.State(); h.PausedTill.After(time.Now()) {
			continue // the crash-loop pause wins over events
		}
		st := events.Load(data)
		ini := config.SettingsPath(r.Cfg.ServerDir)
		want := events.Want(evs, time.Now())
		if st.Switching { // a switch whose restart didn't happen yet (busy, or Warden restarted)
			if r.Restart(ctx, "event switch", nil) == nil {
				st.Switching = false
				events.Save(data, st)
			}
			continue
		}
		switch {
		case want != nil && st.Active != want.Name:
			eff := gamesettings.Effective(ini, data)
			if st.Active != "" { // hand over from another event: put its values back first
				r.restoreNormal(st, eff)
				eff = gamesettings.Effective(ini, data)
			}
			normal := map[string]string{}
			for k := range want.Settings {
				v, ok := eff[k]
				if !ok { // the file doesn't have it yet: the game's default is what it was using
					if o, known := gamesettings.Lookup(k); known {
						v = o.Default
					}
				}
				if v != "" {
					normal[k] = v
				}
			}
			gamesettings.AddPending(data, want.Settings)
			events.Save(data, events.State{Active: want.Name, Normal: normal, Applied: want.Settings, Switching: true})
			r.Store.AddEvent(time.Now(), "event", want.Name+" starts")
			r.Game.Announce(ctx, fmt.Sprintf("%s starts in 5 minutes! The server restarts to switch it on.", want.Name))
			if r.Restart(ctx, "event: "+want.Name, []int{5, 1}) == nil {
				st = events.Load(data)
				st.Switching = false
				events.Save(data, st)
			}
		case want == nil && st.Active != "":
			r.restoreNormal(st, gamesettings.Effective(ini, data))
			events.Save(data, events.State{Switching: true})
			r.Store.AddEvent(time.Now(), "event", st.Active+" ends")
			r.Game.Announce(ctx, fmt.Sprintf("%s ends in 5 minutes. Thanks for playing!", st.Active))
			if r.Restart(ctx, "event over: "+st.Active, []int{5, 1}) == nil {
				events.Save(data, events.State{})
			}
		}
	}
}

// restoreNormal queues the values an event replaced, but only for settings that still hold the
// event's value: anything an admin changed during the event is left as they set it.
func (r *Runner) restoreNormal(st events.State, eff map[string]string) {
	back := map[string]string{}
	for k, normal := range st.Normal {
		if ev, ok := st.Applied[k]; ok && !gamesettings.Same(k, eff[k], ev) {
			continue
		}
		back[k] = normal
	}
	if len(back) > 0 {
		gamesettings.AddPending(r.Backups.DataDir, back)
	}
}
