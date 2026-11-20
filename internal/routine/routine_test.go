package routine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fake struct {
	log                       []string
	alive, stops, backupFails bool
	startCalls                int
}

func (f *fake) Announce(_ context.Context, m string) error {
	f.log = append(f.log, "announce "+m)
	return nil
}
func (f *fake) Save(context.Context) error { f.log = append(f.log, "save"); return nil }
func (f *fake) Shutdown(context.Context, int, string) error {
	f.log = append(f.log, "shutdown")
	return nil
}
func (f *fake) Alive(context.Context) bool { return f.alive }
func (f *fake) WaitStopped(context.Context, time.Duration) bool {
	f.log = append(f.log, "wait")
	return f.stops
}
func (f *fake) ForceStop(context.Context) error {
	f.log = append(f.log, "force")
	f.stops = true
	return nil
}
func (f *fake) Start(context.Context) error {
	f.startCalls++
	f.log = append(f.log, "start")
	f.alive = true
	return nil
}
func (f *fake) Update(context.Context) (string, error) {
	f.log = append(f.log, "update")
	return "none waiting", nil
}
func (f *fake) MakeAll(context.Context) ([]string, error) {
	f.log = append(f.log, "backup")
	if f.backupFails {
		return nil, errors.New("disk full")
	}
	return []string{"pal-data-x.zip", "pal-server-x.zip"}, nil
}
func (f *fake) CopyAndVerify(context.Context, []string) error {
	f.log = append(f.log, "copy")
	return nil
}
func (f *fake) Prune(context.Context) error { f.log = append(f.log, "prune"); return nil }

func plan() Plan {
	return Plan{WarnMinutes: []int{15, 5, 1}, Update: true, HealthWait: time.Second,
		Sleep: func(context.Context, time.Duration) error { return nil }}
}

func TestHappyPath(t *testing.T) {
	f := &fake{stops: true}
	r := Run(context.Background(), plan(), f, f, f)
	if !r.OK {
		t.Fatalf("expected success: %+v", r.Steps)
	}
	got := strings.Join(f.log, "|")
	want := "announce Nightly restart in 15 minutes. The world is saved first.|announce Nightly restart in 5 minutes. The world is saved first.|announce Nightly restart in 1 minute. The world is saved first.|save|shutdown|wait|backup|copy|prune|update|start"
	if got != want {
		t.Fatalf("order:\n got %s\nwant %s", got, want)
	}
}

func TestBackupFailureStillStartsTheServer(t *testing.T) {
	f := &fake{stops: true, backupFails: true}
	r := Run(context.Background(), plan(), f, f, f)
	if r.OK || f.startCalls != 1 || !f.alive {
		t.Fatalf("a failed backup must still start the server and report failure: ok=%v starts=%d", r.OK, f.startCalls)
	}
}

func TestStuckServerIsForceStoppedThenBackedUp(t *testing.T) {
	f := &fake{stops: false}
	r := Run(context.Background(), plan(), f, f, f)
	if !strings.Contains(strings.Join(f.log, "|"), "force|wait|backup") || f.startCalls != 1 {
		t.Fatalf("expected force stop then backup: %v", f.log)
	}
	_ = r
}
