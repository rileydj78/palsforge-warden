package host

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// The script gets the program path and port through environment variables, so no path (quotes,
// Unicode quotes, $ or backticks) can break out of it. Firewall facts come from PowerShell's
// cmdlets (objects, so any Windows language works); only UDP port filters that cover the port
// are looked at, which keeps it quick even with hundreds of rules.
const connScript = `$ErrorActionPreference='SilentlyContinue'
$port = $env:WARDEN_PORT; $exe = $env:WARDEN_EXE
$on = @(Get-NetFirewallProfile | Where-Object Enabled).Count -gt 0
# Players arrive on the adapter that holds the default route: only its profile counts, and rules
# limited to another adapter (e.g. Tailscale-In) don't.
$wan = (Get-NetRoute -DestinationPrefix '0.0.0.0/0' | Sort-Object RouteMetric | Select-Object -First 1).InterfaceIndex
$active = @(Get-NetConnectionProfile -InterfaceIndex $wan | ForEach-Object { [string]$_.NetworkCategory -replace 'DomainAuthenticated','Domain' })
$wanAlias = (Get-NetAdapter -InterfaceIndex $wan).Name
$allowed = $false
$filters = Get-NetFirewallPortFilter -All | Where-Object { ($_.Protocol -eq 'UDP' -or $_.Protocol -eq 'Any') -and ($_.LocalPort -contains $port -or $_.LocalPort -contains 'Any') }
foreach ($r in ($filters | Get-NetFirewallRule | Where-Object { $_.Enabled -eq 'True' -and $_.Direction -eq 'Inbound' -and $_.Action -eq 'Allow' })) {
  $prof = [string]$r.Profile
  if ($prof -ne 'Any' -and -not ($active | Where-Object { $prof -match $_ })) { continue }
  $af = $r | Get-NetFirewallApplicationFilter
  if ($r.Owner -or $af.Package) { continue }
  $sf = $r | Get-NetFirewallServiceFilter
  if ($sf.Service -and $sf.Service -ne 'Any') { continue }
  $if = ($r | Get-NetFirewallInterfaceFilter).InterfaceAlias
  if ($if -and $if -ne 'Any' -and -not ($if -contains $wanAlias)) { continue }
  if ((-not $af.Program) -or ($af.Program -eq 'Any') -or ($exe -and ($af.Program -ieq $exe))) { $allowed = $true; break }
}
$udp = [bool](Get-NetUDPEndpoint -LocalPort $port)
$cl = (Get-CimInstance Win32_Process -OperationTimeoutSec 10 | Where-Object { $_.ExecutablePath -ieq $exe } | Select-Object -First 1).CommandLine
@{ known = $true; firewallOn = $on; allowed = $allowed; listening = $udp; commandLine = [string]$cl } | ConvertTo-Json -Compress`

var connCache struct {
	sync.Mutex
	m map[string]connEntry
}

type connEntry struct {
	f  ConnFacts
	at time.Time
}

// Connection reads the facts (at most 25 seconds; remembered for 2 minutes when they could be
// read). Nothing is locked while PowerShell runs.
func (h *Host) Connection(port int) ConnFacts {
	exe := h.cfg.Abs(h.cfg.Control.Exe)
	key := exe + "|" + strconv.Itoa(port)
	connCache.Lock()
	if c, ok := connCache.m[key]; ok && time.Since(c.at) < 2*time.Minute {
		connCache.Unlock()
		return c.f
	}
	connCache.Unlock()
	f := ConnFacts{LANAddrs: lanAddrs()}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", connScript)
	cmd.Env = append(os.Environ(), "WARDEN_EXE="+exe, "WARDEN_PORT="+strconv.Itoa(port))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := cmd.Output(); err == nil && json.Unmarshal(out, &f) == nil && f.Known {
		f.LANAddrs = lanAddrs()
		connCache.Lock()
		if connCache.m == nil {
			connCache.m = map[string]connEntry{}
		}
		connCache.m[key] = connEntry{f, time.Now()}
		connCache.Unlock()
	}
	return f
}
