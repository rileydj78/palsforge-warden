package nightly

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// AutoPause stops the server after it has been empty for a while (saving power), then listens on
// the game's UDP port and starts it again at the first connection attempt. That first attempt
// fails; the player joins a minute later. A paused server isn't in the community server list, so
// this is for private servers, off by default, and only when Warden runs the server itself.
func (r *Runner) AutoPause(ctx context.Context) {
	minutes := r.Cfg.Health.PauseEmptyMinutes
	if minutes <= 0 || r.Cfg.Control.Mode != "supervise" {
		return
	}
	port := GamePort(r.Cfg.Control.Args)
	var emptySince time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}
		if busy, _ := r.Busy(); busy || !r.Host.State().Wanted {
			emptySince = time.Time{}
			continue
		}
		if n, err := r.playersOnline(ctx); err != nil || n > 0 {
			emptySince = time.Time{}
			continue
		}
		if emptySince.IsZero() {
			emptySince = time.Now()
		}
		if time.Since(emptySince) < time.Duration(minutes)*time.Minute {
			continue
		}
		emptySince = time.Time{}
		if !r.pause(ctx, minutes) {
			continue
		}
		r.waitForKnock(ctx, port)
	}
}

func (r *Runner) pause(ctx context.Context, minutes int) bool {
	end, err := r.Begin("an auto-pause")
	if err != nil {
		return false
	}
	defer end()
	if n, err := r.playersOnline(ctx); err != nil || n > 0 { // someone joined meanwhile
		return false
	}
	r.Game.Save(ctx)
	r.Host.PlanStop(ctx)
	r.Game.Shutdown(ctx, 5, "Pausing: nobody is online.")
	if !r.Host.WaitStopped(ctx, 3*time.Minute) {
		r.Host.ForceStop(ctx)
	}
	r.Store.AddEvent(time.Now(), "pause", fmt.Sprintf("paused: nobody online for %d minutes; it starts again when someone tries to join", minutes))
	r.autoPaused.Store(true)
	return true
}

// waitForKnock listens on the game port until a packet arrives (someone trying to join) or the
// server is started some other way (the nightly routine, an admin), then starts it if needed.
func (r *Runner) waitForKnock(ctx context.Context, port int) {
	defer r.autoPaused.Store(false)
	pc, err := net.ListenPacket("udp", ":"+strconv.Itoa(port))
	if err != nil {
		r.Store.AddEvent(time.Now(), "pause", "couldn't listen on the game port ("+err.Error()+"); starting the server again")
		r.Host.Start(ctx)
		return
	}
	defer pc.Close()
	buf := make([]byte, 1500)
	for {
		if ctx.Err() != nil {
			return
		}
		if r.Host.State().Wanted { // started meanwhile by someone else
			return
		}
		pc.SetReadDeadline(time.Now().Add(5 * time.Second))
		if _, _, err := pc.ReadFrom(buf); err == nil {
			pc.Close() // free the port for the game
			r.Store.AddEvent(time.Now(), "pause", "someone tried to join: starting the server")
			for i := 0; i < 180; i++ { // never start in the middle of an update or a backup
				if end, err := r.Begin("waking the server"); err == nil {
					r.Host.Start(ctx)
					end()
					return
				}
				if r.Host.State().Wanted { // whatever held the lock started it
					return
				}
				time.Sleep(10 * time.Second)
			}
			r.Host.Start(ctx)
			return
		}
	}
}

// GamePort reads -port=NNNN from the server's arguments (the game's default is 8211).
func GamePort(args []string) int {
	for _, a := range args {
		if v, ok := strings.CutPrefix(strings.ToLower(a), "-port="); ok {
			if p, err := strconv.Atoi(v); err == nil && p > 0 && p < 65536 {
				return p
			}
		}
	}
	return 8211
}

func (r *Runner) playersOnline(ctx context.Context) (int, error) {
	g, ok := r.Game.(Game)
	if !ok || g.C == nil {
		return 0, fmt.Errorf("no player count")
	}
	m, err := g.C.Metrics(ctx)
	if err != nil {
		return 0, err
	}
	return m.CurrentPlayers, nil
}

// AutoPaused says whether the server is stopped by auto-pause (it starts when someone joins).
func (r *Runner) AutoPaused() bool { return r.autoPaused.Load() }
