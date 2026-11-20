// Package routine runs the nightly routine: warn players, save, stop, back up, update, copy the
// backups to a second place and check them, start, and confirm the server is healthy.
//
// The rule that matters most: whatever fails, the server is started again at the end. A failed
// backup or update is reported; it never leaves players locked out.
package routine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// Game is what the routine needs from the running server (the REST API).
type Game interface {
	Announce(ctx context.Context, msg string) error
	Save(ctx context.Context) error
	Shutdown(ctx context.Context, waitSeconds int, msg string) error
	Alive(ctx context.Context) bool
}

// Host is what the routine needs from the machine.
type Host interface {
	PlanStop(ctx context.Context) error                    // the coming stop is on purpose: don't restart
	WaitStopped(ctx context.Context, d time.Duration) bool // true once the server process is gone
	ForceStop(ctx context.Context) error
	Start(ctx context.Context) error
	Update(ctx context.Context) (string, error) // runs the game update; returns a short summary
}

// Backups is what the routine needs to make and place backups.
type Backups interface {
	MakeAll(ctx context.Context) ([]string, error) // names of the archives made
	CopyAndVerify(ctx context.Context, names []string) error
	Prune(ctx context.Context) error
}

// Plan says what tonight's routine does.
type Plan struct {
	WarnMinutes []int // e.g. 15, 5, 1: in-game warnings before stopping
	Update      bool
	HealthWait  time.Duration // how long to wait for the server to answer after starting
	Sleep       func(ctx context.Context, d time.Duration) error
	// BeforeStart runs while the server is stopped, just before it starts (e.g. mod changes).
	BeforeStart func(ctx context.Context) (string, error)
}

// Step is one line of the routine's report.
type Step struct {
	At     time.Time `json:"at"`
	Name   string    `json:"name"`
	OK     bool      `json:"ok"`
	Detail string    `json:"detail"`
}

// Report is what happened.
type Report struct {
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	OK       bool      `json:"ok"`
	Steps    []Step    `json:"steps"`
	Backups  []string  `json:"backups"`
}

func (r *Report) add(name string, err error, detail string) bool {
	s := Step{At: time.Now(), Name: name, OK: err == nil, Detail: detail}
	if err != nil {
		if detail != "" {
			s.Detail = detail + ": " + err.Error()
		} else {
			s.Detail = err.Error()
		}
	}
	r.Steps = append(r.Steps, s)
	return err == nil
}

func defaultSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Run does the routine once and always tries to leave the server running.
func Run(ctx context.Context, p Plan, g Game, h Host, b Backups) *Report {
	if p.Sleep == nil {
		p.Sleep = defaultSleep
	}
	if p.HealthWait == 0 {
		p.HealthWait = 5 * time.Minute
	}
	r := &Report{Started: time.Now(), OK: true}
	bad := func(ok bool) {
		if !ok {
			r.OK = false
		}
	}

	// 1. Warnings, longest first, then the remaining wait.
	warn := append([]int(nil), p.WarnMinutes...)
	sort.Sort(sort.Reverse(sort.IntSlice(warn))) // "1, 5, 15" must still mean 15, 5, 1
	warn = slices.Compact(warn)
	for i, m := range warn {
		msg := fmt.Sprintf("Nightly restart in %d minute%s. The world is saved first.", m, map[bool]string{true: "", false: "s"}[m == 1])
		bad(r.add("warn", g.Announce(ctx, msg), fmt.Sprintf("%d min", m)))
		next := 0
		if i+1 < len(warn) {
			next = warn[i+1]
		}
		if err := p.Sleep(ctx, time.Duration(m-next)*time.Minute); err != nil {
			r.add("cancelled", err, "")
			r.OK = false
			r.Finished = time.Now()
			return r
		}
	}

	// 2. Save and stop.
	bad(r.add("save", g.Save(ctx), ""))
	if err := h.PlanStop(ctx); err != nil {
		r.add("plan stop", err, "the watchdog may restart the server early")
	}
	stopped := false
	if err := g.Shutdown(ctx, 10, "Restarting now. Back in a few minutes."); err == nil {
		stopped = h.WaitStopped(ctx, 3*time.Minute)
		r.add("stop", nil, map[bool]string{true: "stopped cleanly", false: "didn't stop in 3 minutes"}[stopped])
	} else {
		r.add("stop", err, "the server didn't accept the shutdown")
	}
	if !stopped {
		err := h.ForceStop(ctx)
		stopped = err == nil && h.WaitStopped(ctx, time.Minute)
		bad(r.add("force stop", err, ""))
	}

	// 3. Back up (only from a stopped server: a running one writes while we copy).
	if stopped {
		names, err := b.MakeAll(ctx)
		r.Backups = names
		bad(r.add("backup", err, strings.Join(names, ", ")))
		if err == nil {
			bad(r.add("copy and check", b.CopyAndVerify(ctx, names), ""))
			r.add("retention", b.Prune(ctx), "")
		}
		// 4. Update.
		if p.Update {
			sum, err := h.Update(ctx)
			bad(r.add("update", err, sum))
		}
	} else {
		r.add("backup", errors.New("skipped"), "the server never stopped")
		r.OK = false
	}

	if stopped && p.BeforeStart != nil {
		sum, err := p.BeforeStart(ctx)
		if sum != "" || err != nil {
			bad(r.add("mods", err, sum))
		}
	}

	// 5. Start, always, and wait until it answers.
	bad(r.add("start", h.Start(ctx), ""))
	deadline := time.Now().Add(p.HealthWait)
	healthy := false
	for time.Now().Before(deadline) {
		if g.Alive(ctx) {
			healthy = true
			break
		}
		if p.Sleep(ctx, 10*time.Second) != nil {
			break
		}
	}
	if healthy {
		r.add("healthy", nil, "the server is answering again")
	} else {
		r.add("healthy", errors.New("the server isn't answering"), fmt.Sprintf("after %s", p.HealthWait))
		r.OK = false
	}
	r.Finished = time.Now()
	return r
}
