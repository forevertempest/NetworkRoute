//go:build windows

package localnet

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestQoSTransactionRollsBackOnlyOwnedRules(t *testing.T) {
	// Execute the actual embedded PowerShell transaction with an in-memory
	// NetQos provider. No system policies or administrator rights are involved.
	mock := `
$script:policies = @{
 'NetworkRoute-test-00' = [pscustomobject]@{Name='NetworkRoute-test-00';AppPathName='old.exe';ThrottleRate=[uint64]12000}
 'Unrelated' = [pscustomobject]@{Name='Unrelated';AppPathName='other.exe';ThrottleRate=[uint64]9000}
}
function Get-NetQosPolicy { param($Name,$PolicyStore,$ErrorAction) if ($Name) { $script:policies[$Name] } else { @($script:policies.Values) } }
function New-NetQosPolicy {
 param($Name,$AppPathNameMatchCondition,$ThrottleRateActionBitsPerSecond,$PolicyStore,$NetworkProfile,$ErrorAction)
 if ($AppPathNameMatchCondition -eq 'fail.exe') { throw 'injected create failure' }
 $script:policies[$Name] = [pscustomobject]@{Name=$Name;AppPathName=$AppPathNameMatchCondition;ThrottleRate=[uint64]$ThrottleRateActionBitsPerSecond}
}
function Remove-NetQosPolicy {
 param($Name,$PolicyStore,$Confirm,$ErrorAction)
 $script:policies.Remove($Name)
}
`
	// Replace process exit only in the test so restored state can be inspected.
	script := strings.ReplaceAll(qosScript, "exit 1", "Write-Output ('RESTORED=' + (ConvertTo-Json -InputObject @($script:policies.Values) -Compress)); return")
	b, err := json.Marshal(map[string]any{"prefix": "NetworkRoute-test-", "action": "apply", "rules": []Rule{{Application: "new.exe", UploadKbps: 200}, {Application: "fail.exe", UploadKbps: 300}}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := runQoSScript(context.Background(), mock+script, b)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "old.exe") || !strings.Contains(got, "other.exe") || !strings.Contains(got, "12000") || strings.Contains(got, "new.exe") {
		t.Fatalf("rollback corrupted policies: %s", got)
	}
}

func TestRealQoSRoundtripOptIn(t *testing.T) {
	if os.Getenv("NETWORKROUTE_TEST_QOS") != "1" {
		t.Skip("opt-in administrative test; never matches a real application")
	}
	dir := t.TempDir()
	defer func() {
		if _, err := QoS(context.Background(), dir, "remove", nil); err != nil {
			t.Error(err)
		}
	}()
	rules := []Rule{{Application: "networkroute-test-nonexistent-7aa857d9.exe", UploadKbps: 80}}
	if _, err := QoS(context.Background(), dir, "apply", rules); err != nil {
		t.Fatal(err)
	}
	got, err := QoS(context.Background(), dir, "status", nil)
	if err != nil || !strings.Contains(got, rules[0].Application) {
		t.Fatalf("missing real policy: %s %v", got, err)
	}
	rules[0].UploadKbps = 160
	if _, err := QoS(context.Background(), dir, "apply", rules); err != nil {
		t.Fatal(err)
	}
	got, err = QoS(context.Background(), dir, "status", nil)
	if err != nil || !strings.Contains(got, "160000") {
		t.Fatalf("update failed: %s %v", got, err)
	}
	if _, err := QoS(context.Background(), dir, "remove", nil); err != nil {
		t.Fatal(err)
	}
	got, err = QoS(context.Background(), dir, "status", nil)
	if err != nil || got != "[]" {
		t.Fatalf("policies leaked: %s %v", got, err)
	}
}
