package host

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/config"
)

func TestParseAffinity(t *testing.T) {
	for in, want := range map[string]uint64{"0": 1, "8-11": 0xF00, "0,2,4": 0b10101, "0-1,4": 0b10011} {
		got, err := ParseAffinity(in)
		if err != nil || got != want {
			t.Fatalf("%s: got %b, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "a", "5-2", "64"} {
		if _, err := ParseAffinity(bad); err == nil {
			t.Fatalf("%q should be refused", bad)
		}
	}
}

func TestBuildID(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "steamapps"), 0o755)
	os.WriteFile(filepath.Join(dir, "steamapps", "appmanifest_2394010.acf"), []byte("\"AppState\"\n{\n\t\"appid\"\t\t\"2394010\"\n\t\"buildid\"\t\t\"20718743\"\n}"), 0o644)
	h := New(config.Defaults(dir), t.TempDir())
	if h.BuildID() != "20718743" {
		t.Fatal(h.BuildID())
	}
}

// A real process: supervise mode starts it, notices it running, and a planned stop keeps it down.
func TestSuperviseStartsAndPlannedStopHolds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a Unix sleep program")
	}
	py, err := exec.LookPath("python3") // a real single-purpose binary (sleep may be a multi-call one)
	if err != nil {
		t.Skip("no python3")
	}
	dir := t.TempDir()
	prog := filepath.Join(dir, "fakepalserver")
	b, _ := os.ReadFile(py)
	os.WriteFile(prog, b, 0o755)
	cfg := config.Defaults(dir)
	cfg.Control.Exe, cfg.Control.Args = prog, []string{"-c", "import time; time.sleep(30)"}
	h := New(cfg, t.TempDir())
	var events []string
	h.OnEvent = func(e Event) { events = append(events, e.Kind) }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := h.Start(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if !h.Running() {
		t.Fatal("the server should be running")
	}
	if h.Memory() <= 0 {
		t.Fatal("a running server's memory should be measurable")
	}
	go h.Supervise(ctx)
	h.PlanStop(ctx)
	if err := h.ForceStop(ctx); err != nil {
		t.Fatal(err)
	}
	if !h.WaitStopped(ctx, 10*time.Second) {
		t.Fatal("it should have stopped")
	}
	time.Sleep(6 * time.Second) // the supervisor must not bring it back after a planned stop
	if h.Running() {
		t.Fatal("a planned stop must hold")
	}
	h.Start(ctx)
	time.Sleep(300 * time.Millisecond)
	if !h.Running() {
		t.Fatal("start after a planned stop should work")
	}
	h.ForceStop(ctx)
	h.WaitStopped(ctx, 10*time.Second)
}

func TestExternalStopMarker(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Defaults(dir)
	cfg.Control.Mode = "external"
	cfg.Control.StopMarker = "STOP"
	cfg.Control.StartCommand = []string{"true"}
	if runtime.GOOS == "windows" {
		cfg.Control.StartCommand = []string{"cmd", "/c", "exit", "0"}
	}
	h := New(cfg, t.TempDir())
	h.PlanStop(context.Background())
	if _, err := os.Stat(filepath.Join(dir, "STOP")); err != nil {
		t.Fatal("a planned stop must create the marker")
	}
	if err := h.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "STOP")); !os.IsNotExist(err) {
		t.Fatal("start must remove the marker")
	}
}

func TestPublicBuildIsRead(t *testing.T) {
	out := `"2394010"
{
	"depots"
	{
		"branches"
		{
			"public"
			{
				"buildid"		"20841234"
				"timeupdated"		"1795000000"
			}
		}
	}
}`
	m := publicBuildRe.FindStringSubmatch(out)
	if m == nil || m[1] != "20841234" {
		t.Fatalf("got %v", m)
	}
}

func TestOnlyThisServersProcessCounts(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("uses /proc")
	}
	// Two copies of the same program in different folders: only the one asked about counts.
	dir := t.TempDir()
	b, err := os.ReadFile("/bin/sh") // a real program that runs under any name
	if err != nil {
		t.Skip("no /bin/sh")
	}
	a, c := filepath.Join(dir, "a", "PalServer"), filepath.Join(dir, "b", "PalServer")
	for _, p := range []string{a, c} {
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, b, 0o755)
	}
	cmd := exec.Command(a, "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Skip(err)
	}
	defer cmd.Process.Kill()
	time.Sleep(200 * time.Millisecond)
	if p, _ := pidsOf(a); len(p) != 1 {
		t.Fatalf("its own server: %v", p)
	}
	if p, _ := pidsOf(c); len(p) != 0 {
		t.Fatalf("another server with the same program name must not count: %v", p)
	}
}
