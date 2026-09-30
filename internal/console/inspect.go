package console

import (
	"context"
	"fmt"
	"net"
	"networkroute/internal/config"
	"networkroute/internal/engine"
	"networkroute/internal/monitor"
	"networkroute/internal/onboarding"
	"networkroute/internal/policy"
	"networkroute/internal/quality"
	"sort"
	"strings"
	"time"
)

func (a *App) network() error {
	a.header("Сеть и приложения")
	interfaces, err := monitor.Interfaces()
	if err != nil {
		return err
	}
	for _, i := range interfaces {
		if !strings.Contains(i.Flags, "up") {
			continue
		}
		a.print("  %s · MTU %d\n    %s\n", safe(i.Name), i.MTU, safe(strings.Join(i.Addresses, ", ")))
	}
	hints, err := monitor.VPNHints()
	if err != nil {
		return err
	}
	if len(hints) > 0 {
		a.print("\n  Возможные VPN/туннели: %s\n", safe(strings.Join(hints, ", ")))
	}
	filter, err := a.read("Фильтр процесса/PID/адреса (Enter — все):")
	if err != nil {
		return err
	}
	rows, err := monitor.Connections()
	if err != nil {
		return err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].PID < rows[j].PID })
	a.print("\n  PID      ПРОЦЕСС                  ТИП     НАЗНАЧЕНИЕ\n")
	count := 0
	for _, r := range rows {
		line := fmt.Sprintf("%d %s %s %s", r.PID, r.Process, r.Remote, r.Local)
		if filter != "" && !strings.Contains(strings.ToLower(line), strings.ToLower(filter)) {
			continue
		}
		remote := r.Remote
		if remote == "" {
			remote = "local " + r.Local + " (remote недоступен в API)"
		}
		process := []rune(safe(r.Process))
		if len(process) > 23 {
			process = append(process[:22], '…')
		}
		a.print("  %-8d %-24s %-7s %s\n", r.PID, string(process), safe(r.Protocol), safe(remote))
		count++
		if count%18 == 0 {
			answer, e := a.read("Enter — дальше; 0 — закончить:")
			if e != nil {
				return e
			}
			if answer == "0" {
				break
			}
		}
	}
	a.print("\n  Показано: %d. Это наблюдение, а не прозрачный перехват процессов.\n", count)
	return nil
}
func (a *App) doctor() error {
	a.header("Проверка подключения")
	a.print("  Проверяю конфигурацию, порт, VPN и handshake настроенного relay…\n")
	report := onboarding.Doctor(a.ctx, a.configPath, true)
	for _, c := range report.Checks {
		color := "92"
		if c.Status == "warning" {
			color = "93"
		}
		if c.Status == "failed" {
			color = "91"
		}
		a.print("\n  %s %s\n    %s\n", a.tint(color, "["+c.Status+"]"), safe(c.Name), safe(c.Detail))
	}
	if a.session != nil {
		a.print("\n  Занятый SOCKS-порт ожидаем, когда ядро уже запущено.\n")
	}
	a.print("\n  Результат: %s. Handshake не является измерением RTT до сайта.\n", report.Status)
	return nil
}
func (a *App) statsRow(name string, s quality.Stats) {
	if s.Successes == 0 {
		a.print("  %-18s нет успешных подключений (%d проб)\n", safe(name), s.Samples)
		return
	}
	a.print("  %-18s %8.2f %8.2f %8.2f   %d/%d\n", safe(name), s.Median, s.P95, s.P99, s.Successes, s.Samples)
}
func (a *App) benchmark() error {
	a.header("Измерение качества пути")
	a.print("  Измеряем время TCP connect до назначения, не игровой ping.\n  Будут созданы короткие соединения; payload не отправляется.\n")
	target, err := a.read("Назначение host:port (Enter — example.com:443):")
	if err != nil {
		return err
	}
	if target == "" {
		target = "example.com:443"
	}
	c, err := config.Load(a.configPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	resolve, stop := context.WithTimeout(ctx, 3*time.Second)
	addresses, err := policy.Resolve(resolve, net.DefaultResolver, target)
	stop()
	if err != nil {
		return err
	}
	relay, err := engine.Relay(c)
	if err != nil {
		return err
	}
	a.print("\n  Измерение: до 30 секунд, максимум 5 проб на путь…\n")
	if relay == nil {
		// Limit destinations so a DNS response cannot turn a quick check into a scan.
		if len(addresses) > 4 {
			addresses = addresses[:4]
		}
		report := quality.MeasureDirect(ctx, addresses, 5, 2*time.Second, quality.NativeDial)
		a.print("\n  Путь                 Median      p95      p99   Успех\n")
		for _, r := range report.Results {
			a.statsRow(r.Target, r.Quality)
		}
		a.print("\n  Использован текущий путь ОС. Relay не настроен.\n")
	} else {
		defer relay.Close()
		handshake, stop := context.WithTimeout(ctx, 4*time.Second)
		err = relay.Connect(handshake)
		stop()
		if err != nil {
			return err
		}
		report := quality.Measure(ctx, addresses[0], 5, 2*time.Second, quality.NativeDial, relay.DialTCP)
		a.print("\n  Один endpoint: %s\n  Путь                 Median      p95      p99   Успех\n", safe(addresses[0]))
		a.statsRow("Direct", report.Direct)
		a.statsRow("Relay", report.Relay)
		if report.Direct.Successes == 0 && report.Relay.Successes == 0 {
			a.print("\n  Нет достаточных данных. Проверьте доступность назначения.\n")
		} else if report.Relay.Successes >= 5 && quality.Score(report.Relay, c.Profile)+5 < quality.Score(report.Direct, c.Profile) && quality.Score(report.Relay, c.Profile) < 0.85*quality.Score(report.Direct, c.Profile) {
			a.print("\n  Relay — кандидат. Smart ещё проверит устойчивость выигрыша.\n")
		} else {
			a.print("\n  Устойчивое преимущество relay не установлено; Direct сохраняется.\n")
		}
	}
	if ctx.Err() != nil {
		a.print("  Лимит времени достигнут: результаты частичные.\n")
	}
	a.print("  Единицы: ms. Отказы connect не равны packet loss.\n  При активном VPN Direct означает маршрут ОС через этот VPN.\n")
	return nil
}
func (a *App) statistics() error {
	a.header("Счётчики и журнал текущего запуска")
	if a.session == nil {
		a.print("  Ядро остановлено.\n")
	} else {
		m := a.session.Router.Metrics
		a.print("  Активные потоки    %d\n  Всего Direct       %d\n  Всего Relay        %d\n  Передано           %.2f MiB\n  UDP датаграмм      %d\n  Отброшено          %d\n  Fallback новых     %d\n", m.Active.Load(), m.Direct.Load(), m.Relayed.Load(), float64(m.Bytes.Load())/1048576, m.Datagrams.Load(), m.Dropped.Load(), m.Fallbacks.Load())
	}
	a.print("\n  Журнал INFO/WARN/ERROR, последние 32 KiB, только в памяти:\n")
	for _, line := range strings.Split(a.logs.String(), "\n") {
		if line != "" {
			a.print("  %s\n", safe(line))
		}
	}
	return nil
}
