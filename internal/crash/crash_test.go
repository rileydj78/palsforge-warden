package crash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFindsAModOnTheStack(t *testing.T) {
	srv := t.TempDir()
	since := time.Now().Add(-time.Minute)
	cr := filepath.Join(srv, "Pal", "Saved", "Crashes", "UECC-Windows-ABC")
	os.MkdirAll(cr, 0o755)
	os.WriteFile(filepath.Join(cr, "CrashContext.runtime-xml"), []byte(`<?xml version="1.0"?><FGenericCrashContext><RuntimeProperties>
<CrashType>Crash</CrashType><ErrorMessage>Unhandled Exception: EXCEPTION_ACCESS_VIOLATION reading address 0x0000000000000010
second line</ErrorMessage>
<CallStack>PalServer-Win64-Shipping!UObject::ProcessEvent()
UE4SS!RC::Unreal::Hook::ProcessEvent()
PalServer-Win64-Shipping!FEngineLoop::Tick()</CallStack></RuntimeProperties></FGenericCrashContext>`), 0o644)
	h := Find(srv, filepath.Join("Pal", "Binaries", "Win64"), since)
	if !strings.Contains(h.Summary, "UE4SS") || !strings.Contains(h.Summary, "ACCESS_VIOLATION") {
		t.Fatalf("%+v", h)
	}
	if h2 := Find(srv, "x", time.Now().Add(time.Hour)); h2.Summary != "" {
		t.Fatal("an old crash report is not this crash")
	}
}

func TestMemoryAndModLogs(t *testing.T) {
	srv := t.TempDir()
	logs := filepath.Join(srv, "Pal", "Saved", "Logs")
	os.MkdirAll(logs, 0o755)
	os.WriteFile(filepath.Join(logs, "Pal.log"), []byte("LogTemp: fine\nLogMemory: Fatal error: Ran out of memory allocating 1048576 bytes\n"), 0o644)
	h := Find(srv, "x", time.Now().Add(-time.Minute))
	if !strings.Contains(h.Summary, "memory") {
		t.Fatalf("%+v", h)
	}
	srv2 := t.TempDir()
	u := filepath.Join(srv2, "Pal", "Binaries", "Win64", "ue4ss")
	os.MkdirAll(u, 0o755)
	os.WriteFile(filepath.Join(u, "UE4SS.log"), []byte("[12:00:00] [Lua] [BetterChests] Error: attempt to index a nil value\n"), 0o644)
	h = Find(srv2, filepath.Join("Pal", "Binaries", "Win64"), time.Now().Add(-time.Minute))
	if !strings.Contains(h.Summary, "BetterChests") {
		t.Fatalf("%+v", h)
	}
	if h := Find(t.TempDir(), "x", time.Now()); h.Summary != "" {
		t.Fatal("nothing found, nothing said")
	}
}
