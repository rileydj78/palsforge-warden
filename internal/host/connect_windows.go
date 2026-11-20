package host

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Connection reads the facts with PowerShell's firewall cmdlets (objects, so any Windows
// language works) and the running server's command line.
func (h *Host) Connection(port int) ConnFacts {
	connCache.Lock()
	defer connCache.Unlock()
	if c, ok := connCache.m[port]; ok && time.Since(c.at) < 2*time.Minute { // the cmdlets take seconds
		return c.f
	}
	f := ConnFacts{LANAddrs: lanAddrs()}
	exe := h.cfg.Abs(h.cfg.Control.Exe)
	ps := fmt.Sprintf(`$ErrorActionPreference='SilentlyContinue'
$port = '%d'; $exe = '%s'
$on = @(Get-NetFirewallProfile | Where-Object Enabled).Count -gt 0
# A rule lets players in only if BOTH its port and its program fit (a rule for Steam on any
# port doesn't count).
$allowed = $false
# Rules for Windows' own app packages or services don't count, and only rules for an active
# profile (Domain/Private/Public) apply.
# Players arrive on the adapter that holds the default route, so only that adapter's profile
# counts, and rules limited to another adapter (e.g. Tailscale-In) don't.
$wan = (Get-NetRoute -DestinationPrefix '0.0.0.0/0' | Sort-Object RouteMetric | Select-Object -First 1).InterfaceIndex
$active = @(Get-NetConnectionProfile -InterfaceIndex $wan | ForEach-Object { [string]$_.NetworkCategory -replace 'DomainAuthenticated','Domain' })
$wanAlias = (Get-NetAdapter -InterfaceIndex $wan).Name
foreach ($r in Get-NetFirewallRule -Direction Inbound -Enabled True -Action Allow) {
  $prof = [string]$r.Profile
  if ($prof -ne 'Any' -and -not ($active | Where-Object { $prof -match $_ })) { continue }
  if ($r.Owner -or ($r | Get-NetFirewallApplicationFilter).Package) { continue }
  $if = ($r | Get-NetFirewallInterfaceFilter).InterfaceAlias
  if ($if -and $if -ne 'Any' -and -not ($if -contains $wanAlias)) { continue }
  $sf = $r | Get-NetFirewallServiceFilter
  if ($sf.Service -and $sf.Service -ne 'Any') { continue }
  $pf = $r | Get-NetFirewallPortFilter; $af = $r | Get-NetFirewallApplicationFilter
  $portOk = ($pf.Protocol -eq 'UDP' -or $pf.Protocol -eq 'Any') -and ($pf.LocalPort -contains $port -or $pf.LocalPort -contains 'Any')
  $progOk = (-not $af.Program) -or ($af.Program -eq 'Any') -or ($exe -and ($af.Program -ieq $exe))
  if ($portOk -and $progOk) { $allowed = $true; break }
}
$udp = [bool](Get-NetUDPEndpoint -LocalPort $port)
$cl = (Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -ieq $exe } | Select-Object -First 1).CommandLine
if (-not $cl) { $cl = (Get-CimInstance Win32_Process -Filter "Name like 'PalServer%%'" | Select-Object -First 1).CommandLine }
@{ known = $true; firewallOn = $on; allowed = $allowed; listening = $udp; commandLine = [string]$cl } | ConvertTo-Json -Compress`, port, escapePS(exe))
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err == nil {
		json.Unmarshal(out, &f)
	}
	f.LANAddrs = lanAddrs()
	if connCache.m == nil {
		connCache.m = map[int]struct {
			f  ConnFacts
			at time.Time
		}{}
	}
	connCache.m[port] = struct {
		f  ConnFacts
		at time.Time
	}{f, time.Now()}
	return f
}

var connCache struct {
	sync.Mutex
	m map[int]struct {
		f  ConnFacts
		at time.Time
	}
}

func escapePS(s string) string {
	b := []byte{}
	for _, c := range []byte(s) {
		if c == '\'' {
			b = append(b, '\'')
		}
		b = append(b, c)
	}
	return string(b)
}
