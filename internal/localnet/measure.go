package localnet

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"networkroute/internal/monitor"
	"networkroute/internal/onboarding"
	"networkroute/internal/policy"
	"networkroute/internal/quality"
	"networkroute/internal/storage"
	"path/filepath"
	"time"
)

type Report struct {
	Version  int           `json:"version"`
	At       time.Time     `json:"at"`
	Target   string        `json:"target"`
	Endpoint string        `json:"endpoint"`
	Metric   string        `json:"metric"`
	Network  string        `json:"network_fingerprint"`
	Settings Settings      `json:"settings"`
	Stats    quality.Stats `json:"stats"`
}

// A pinned endpoint makes the before/after test comparable even if DNS changes.
func Measure(ctx context.Context, s Settings, endpoint string) (Report, error) {
	if err := s.Validate(); err != nil {
		return Report{}, err
	}
	if endpoint == "" {
		rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		addrs, err := policy.Resolve(rctx, net.DefaultResolver, s.Target)
		cancel()
		if err != nil {
			return Report{}, err
		}
		endpoint = addrs[0]
	}
	fingerprint, err := monitor.Fingerprint()
	if err != nil {
		return Report{}, err
	}
	r := Report{Version: 1, At: time.Now(), Target: s.Target, Endpoint: endpoint, Metric: "TCP connect time, not ICMP/game RTT, packet loss or throughput", Network: fingerprint, Settings: s}
	samples := make([]quality.Sample, 0, s.Samples)
	for i := 0; i < s.Samples; i++ {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		pctx, cancel := context.WithTimeout(ctx, time.Duration(s.TimeoutMS)*time.Millisecond)
		at := time.Now()
		conn, err := quality.NativeDial(pctx, endpoint)
		elapsed := time.Since(at)
		cancel()
		if conn != nil {
			conn.Close()
		}
		samples = append(samples, quality.Sample{Millis: float64(elapsed) / float64(time.Millisecond), OK: err == nil, At: at})
		if i+1 < s.Samples {
			select {
			case <-ctx.Done():
				return Report{}, ctx.Err()
			case <-time.After(time.Duration(s.IntervalMS) * time.Millisecond):
			}
		}
	}
	if err := ctx.Err(); err != nil { return Report{}, err }
	endFingerprint, err := monitor.Fingerprint()
	if err != nil { return Report{}, err }
	if endFingerprint != fingerprint { return Report{}, fmt.Errorf("сеть изменилась во время измерения; повторите замер") }
	r.Stats = quality.Summarize(samples)
	return r, nil
}

func SaveReport(dir, name string, r Report) error {
	if name != "before" && name != "after" {
		return fmt.Errorf("invalid report name")
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return storage.Write(filepath.Join(dir, "reports", name+".json"), append(b, '\n'), nil)
}

func LoadBaseline(dir string) (Report, error) {
	var r Report
	err := onboarding.ReadJSON(filepath.Join(dir, "reports", "before.json"), &r)
	if err != nil {
		return r, err
	}
	h, p, err := net.SplitHostPort(r.Endpoint)
	if err != nil || net.ParseIP(h) == nil || p == "" || r.Version != 1 {
		return r, fmt.Errorf("invalid baseline endpoint/version")
	}
	if err := r.Settings.Validate(); err != nil {
		return r, err
	}
	if r.Stats.Samples < 5 || r.Stats.Successes == 0 {
		return r, fmt.Errorf("baseline has insufficient successful observations")
	}
	return r, nil
}

func Compare(before, after Report) string {
	if before.Endpoint != after.Endpoint || before.Target != after.Target || before.Network != after.Network || before.Settings.Samples != after.Settings.Samples || before.Settings.TimeoutMS != after.Settings.TimeoutMS || before.Settings.IntervalMS != after.Settings.IntervalMS {
		return "Несопоставимые условия: изменились endpoint, сеть или параметры проб. Повторите исходный замер."
	}
	if before.Stats.Successes < 5 || after.Stats.Successes < 5 {
		return "Недостаточно успешных соединений для сравнения."
	}
	if after.Stats.FailureRate > before.Stats.FailureRate || after.Stats.Median > before.Stats.Median*1.15+2 || after.Stats.P95 > before.Stats.P95*1.15+2 {
		return "Наблюдается ухудшение. Повторите замер при одинаковой нагрузке; активные ограничения можно снять пунктом 5."
	}
	if after.Stats.Median+2 < before.Stats.Median*.85 && after.Stats.P95 <= before.Stats.P95 && after.Stats.FailureRate <= before.Stats.FailureRate {
		return "В этом замере TCP connect стал быстрее. Повторите проверку: причинность и улучшение игрового RTT не установлены."
	}
	return "Убедительного улучшения в этом замере нет."
}
