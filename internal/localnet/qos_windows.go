//go:build windows

package localnet

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// The script is fixed code; application names are JSON data, never PowerShell code.
// Only this installation's exact policy names are touched. ActiveStore expires on reboot.
const qosScript = `
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
try {
 $request = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($env:NETWORKROUTE_QOS_REQUEST)) | ConvertFrom-Json
 $all = @(Get-NetQosPolicy -PolicyStore ActiveStore -ErrorAction Stop)
 $owned = @($all | Where-Object { $_.Name -cmatch ('^' + [regex]::Escape($request.prefix) + '[0-9]{2}$') })
 if ($request.action -eq 'status') {
  ConvertTo-Json -InputObject @($owned | Select-Object Name,AppPathName,ThrottleRate) -Depth 4 -Compress
  exit 0
 }
 $old = @($owned | ForEach-Object { @{name=$_.Name; app=$_.AppPathName; rate=[uint64]$_.ThrottleRate} })
 $created = @()
 try {
  foreach ($p in $owned) { Remove-NetQosPolicy -Name $p.Name -PolicyStore ActiveStore -Confirm:$false -ErrorAction Stop }
  if ($request.action -eq 'apply') {
   $i=0
   foreach ($r in $request.rules) {
    $name = $request.prefix + $i.ToString('D2')
    New-NetQosPolicy -Name $name -AppPathNameMatchCondition $r.application -ThrottleRateActionBitsPerSecond ([uint64]$r.upload_kbps * 1000) -PolicyStore ActiveStore -NetworkProfile All -ErrorAction Stop | Out-Null
    $created += $name
    $i++
   }
  }
 } catch {
  $original = $_
  foreach ($name in $created) { Remove-NetQosPolicy -Name $name -PolicyStore ActiveStore -Confirm:$false -ErrorAction Continue }
  foreach ($p in $old) {
   if (-not (Get-NetQosPolicy -Name $p.name -PolicyStore ActiveStore -ErrorAction SilentlyContinue)) {
    New-NetQosPolicy -Name $p.name -AppPathNameMatchCondition $p.app -ThrottleRateActionBitsPerSecond $p.rate -PolicyStore ActiveStore -NetworkProfile All -ErrorAction Continue | Out-Null
   }
  }
  throw $original
 }
 Write-Output 'OK'
} catch { [Console]::Error.WriteLine($_.Exception.Message); exit 1 }
`

func QoS(ctx context.Context, dir, action string, rules []Rule) (string, error) {
	if action != "status" && action != "apply" && action != "remove" {
		return "", fmt.Errorf("invalid QoS action")
	}
	if action != "status" && !windows.GetCurrentProcessToken().IsElevated() {
		return "", fmt.Errorf("для применения/снятия лимитов запустите NetworkRoute от имени администратора")
	}
	s := Defaults()
	s.Rules = rules
	if err := s.Validate(); err != nil {
		return "", err
	}
	if action == "apply" && len(rules) == 0 {
		return "", fmt.Errorf("сначала добавьте ограничения приложений")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(abs))))
	prefix := fmt.Sprintf("NetworkRoute-%x-", hash[:8])
	if err := os.MkdirAll(abs, 0700); err != nil { return "", err }
	lockPath, err := windows.UTF16PtrFromString(filepath.Join(abs, "qos.lock"))
	if err != nil { return "", err }
	lock, err := windows.CreateFile(lockPath, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil { return "", fmt.Errorf("другая операция QoS выполняется или файл блокировки недоступен: %w", err) }
	defer windows.CloseHandle(lock)
	b, err := json.Marshal(map[string]any{"action": action, "prefix": prefix, "rules": rules})
	if err != nil {
		return "", err
	}
	return runQoSScript(ctx, qosScript, b)
}

func runQoSScript(ctx context.Context, script string, request []byte) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	ps := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.CommandContext(ctx, ps, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Env = append(os.Environ(), "NETWORKROUTE_QOS_REQUEST="+base64.StdEncoding.EncodeToString(request))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("NetQos: %s (%w)", strings.TrimSpace(stderr.String()), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}
