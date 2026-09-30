package console

import (
	"fmt"
	"networkroute/internal/localnet"
	"networkroute/internal/monitor"
	"networkroute/internal/startup"
	"path/filepath"
	"strconv"
	"strings"
)

func (a *App) localControls() error {
	s, err := localnet.Load(a.dir)
	if err != nil {
		return err
	}
	a.header("Локальное управление и проверка результата")
	a.print("  Цель: %s · %d проб · timeout %d ms · интервал %d ms\n", safe(s.Target), s.Samples, s.TimeoutMS, s.IntervalMS)
	a.print("  Сохранённые ограничения исходящего трафика (kbit/s):\n")
	for i, r := range s.Rules {
		a.print("    %d. %s: %d\n", i+1, safe(r.Application), r.UploadKbps)
	}
	if len(s.Rules) == 0 {
		a.print("    нет\n")
	}
	a.print("\n  [1] Настроить измерения          [2] Добавить / изменить лимит .exe\n  [3] Удалить сохранённый лимит   [4] Применить лимиты Windows\n  [5] Снять действующие лимиты    [6] Действующие правила Windows\n  [7] Измерить ДО                 [8] Измерить ПОСЛЕ и сравнить\n  [9] Загрузка сетевых адаптеров  [0] Назад\n\n  Лимиты работают для исходящего TCP/UDP выбранных .exe без SOCKS/VPS.\n  Применение/снятие требует администратора. Они действуют до снятия\n  или перезагрузки, в том числе после закрытия программы.\n  Сохранение списка само по себе не меняет действующие правила.\n  Это ограничение фоновой отправки, не увеличение скорости канала.\n")
	choice, err := a.read("Пункт:")
	if err != nil {
		return err
	}
	switch choice {
	case "0", "":
		return nil
	case "1":
		if s.Target, err = a.read("Цель host:port:"); err != nil {
			return err
		}
		for _, item := range []struct {
			prompt string
			dst    *int
		}{{"Число проб (5–60):", &s.Samples}, {"Таймаут одной пробы, ms (100–5000):", &s.TimeoutMS}, {"Интервал, ms (50–5000):", &s.IntervalMS}} {
			value, e := a.read(item.prompt)
			if e != nil {
				return e
			}
			n, e := strconv.Atoi(value)
			if e != nil {
				return fmt.Errorf("ожидается целое число")
			}
			*item.dst = n
		}
	case "2":
		app, e := a.read("Имя .exe или полный путь (например backup.exe):")
		if e != nil {
			return e
		}
		app = strings.Trim(app, "\"")
		value, e := a.read("Лимит отправки в kbit/s (1000 = 1 Mbit/s):")
		if e != nil {
			return e
		}
		n, e := strconv.Atoi(value)
		if e != nil {
			return e
		}
		found := false
		for i := range s.Rules {
			if strings.EqualFold(s.Rules[i].Application, app) {
				s.Rules[i].UploadKbps = n
				found = true
			}
		}
		if !found {
			s.Rules = append(s.Rules, localnet.Rule{Application: app, UploadKbps: n})
		}
	case "3":
		value, e := a.read("Номер сохранённого правила:")
		if e != nil {
			return e
		}
		n, e := strconv.Atoi(value)
		if e != nil || n < 1 || n > len(s.Rules) {
			return fmt.Errorf("нет такого правила")
		}
		s.Rules = append(s.Rules[:n-1], s.Rules[n:]...)
	case "4", "5", "6":
		action := map[string]string{"4": "apply", "5": "remove", "6": "status"}[choice]
		result, e := localnet.QoS(a.ctx, a.dir, action, s.Rules)
		if e != nil {
			return e
		}
		a.print("\n  %s\n", safe(result))
		return nil
	case "7", "8":
		phase, endpoint := "before", ""
		var before localnet.Report
		if choice == "8" {
			before, err = localnet.LoadBaseline(a.dir)
			if err != nil {
				return fmt.Errorf("сначала выполните успешный замер ДО: %w", err)
			}
			if s.Target != before.Target {
				return fmt.Errorf("цель изменилась; повторите замер ДО")
			}
			phase, endpoint = "after", before.Endpoint
		}
		a.print("\n  Выполняю %d TCP connect-проб. Поддерживайте одинаковую нагрузку\n  для замеров ДО/ПОСЛЕ. Это не игровой ping и не тест скорости.\n", s.Samples)
		r, e := localnet.Measure(a.ctx, s, endpoint)
		if e != nil {
			return e
		}
		if e = localnet.SaveReport(a.dir, phase, r); e != nil {
			return e
		}
		a.print("\n  Endpoint: %s\n  Путь                 Median      p95      p99   Успех\n", safe(r.Endpoint))
		if choice == "8" {
			a.statsRow("ДО", before.Stats)
		}
		a.statsRow(strings.ToUpper(phase), r.Stats)
		if choice == "8" {
			a.print("\n  %s\n", localnet.Compare(before, r))
		}
		a.print("\n  Отчёт: %s\n", safe(filepath.Join(a.dir, "reports", phase+".json")))
		return nil
	case "9":
		a.print("\n  Измеряю фактическую загрузку интерфейсов за 1 секунду…\n")
		rows, e := monitor.TrafficRates(a.ctx)
		if e != nil {
			return e
		}
		for _, r := range rows {
			a.print("  %s: вход %.3f Mbit/s · выход %.3f Mbit/s\n", safe(r.Name), r.ReceiveMbps, r.SendMbps)
		}
		a.print("  Это текущий трафик интерфейса, не максимальная скорость интернета.\n")
		return nil
	default:
		return fmt.Errorf("неизвестный пункт")
	}
	if err = localnet.Save(a.dir, s); err != nil {
		return err
	}
	a.print("\n  Сохранено. Для изменения действующих лимитов используйте пункт 4.\n")
	return nil
}

func (a *App) startupSettings() error {
	s, err := localnet.Load(a.dir)
	if err != nil {
		return err
	}
	state, err := startup.Status(a.dir)
	if err != nil {
		return err
	}
	a.header("Запуск программы")
	a.print("  Автозапуск: %t — %s\n  Включать прокси при обычном открытии: %t\n", state.Enabled, state.When, s.StartOnLaunch)
	if state.Enabled {
		a.print("  %s\n", safe(state.Command))
	}
	a.print("\n  [1] Включить / обновить автозапуск\n  [2] Отключить автозапуск\n  [3] Переключить запуск прокси при обычном открытии\n  [0] Назад\n\n  Автозапуск открывает меню и сразу запускает SOCKS-прокси.\n  Самостоятельная копия программы сохраняется в каталоге данных/app.\n  Исходный EXE/репозиторий после этого не нужны. Закрытие окна\n  останавливает прокси. Это запуск при входе, а не служба до входа.\n  Для обновления копии откройте новую версию и выберите пункт 1.\n")
	choice, err := a.read("Пункт:")
	if err != nil {
		return err
	}
	switch choice {
	case "0", "":
		return nil
	case "1":
		err = startup.Enable(a.dir)
	case "2":
		err = startup.Disable(a.dir)
	case "3":
		s.StartOnLaunch = !s.StartOnLaunch
		err = localnet.Save(a.dir, s)
	default:
		return fmt.Errorf("неизвестный пункт")
	}
	if err == nil {
		a.print("\n  Настройка запуска сохранена.\n")
	}
	return err
}
