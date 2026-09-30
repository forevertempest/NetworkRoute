package console

import (
	"fmt"
	"net"
	"networkroute/internal/config"
	"networkroute/internal/policy"
	"strconv"
	"strings"
)

func (a *App) settings() error {
	if err := a.requireStopped(); err != nil {
		return err
	}
	c, err := config.Load(a.configPath)
	if err != nil {
		return err
	}
	a.header("Настройки")
	a.print("  [1] Режим: %s\n  [2] Профиль трафика: %s\n  [3] Порт SOCKS5: %s\n  [4] При отказе relay: %s\n  [5] Relay поверх VPN: %t\n  [6] Исключения: домены\n  [7] Исключения: IP/CIDR\n  [8] Лимит подключений: %d\n  [9] Интервал TCP IPv4/IPv6: %d ms\n  [0] Назад\n", safe(c.Mode), safe(string(c.Profile)), safe(c.Listen), safe(c.FailMode), c.AllowNestedTunnel, c.MaxConnections, c.DirectRaceDelayMS)
	choice, err := a.read("Пункт:")
	if err != nil {
		return err
	}
	var value string
	switch choice {
	case "0", "":
		return nil
	case "1":
		a.print("\n  smart — измерять и выбирать; direct — текущий путь ОС;\n  relay — принудительно через настроенный VPS. Улучшение не гарантируется.\n")
		value, err = a.read("Режим (smart/direct/relay):")
		c.Mode = strings.ToLower(value)
	case "2":
		a.print("\n  AUTO / REALTIME / INTERACTIVE / STREAMING / BULK\n  AUTO сейчас = INTERACTIVE. STREAMING/BULK в Smart сохраняют Direct.\n")
		value, err = a.read("Профиль:")
		c.Profile = policy.Profile(strings.ToUpper(value))
	case "3":
		value, err = a.read("Локальный порт (1024–65535):")
		if err != nil {
			return err
		}
		port, e := strconv.Atoi(value)
		if e != nil || port < 1024 || port > 65535 {
			return fmt.Errorf("порт должен быть 1024–65535")
		}
		c.Listen = net.JoinHostPort("127.0.0.1", value)
	case "4":
		a.print("\n  open — новые соединения могут использовать Direct при отказе relay;\n  closed — eligible-потоки без relay блокируются. LAN остаётся Direct.\n  Это правило прокси, а не системный VPN kill switch.\n")
		value, err = a.read("Поведение (open/closed):")
		c.FailMode = strings.ToLower(value)
	case "5":
		a.print("\n  По умолчанию обнаруженный VPN приостанавливает relay.\n  Вложенный relay не обходит VPN kill switch и может увеличить задержку.\n")
		value, err = a.read("Разрешить вложенный relay? (да/нет):")
		if value != "да" && value != "нет" {
			return fmt.Errorf("введите да или нет")
		}
		c.AllowNestedTunnel = value == "да"
	case "6", "7":
		old := c.BypassDomains
		if choice == "7" {
			old = c.BypassCIDRs
		}
		a.print("\n  Сейчас: %s\n  Новый полный список через запятую; '-' — очистить.\n", safe(strings.Join(old, ", ")))
		value, err = a.read("Список:")
		if value == "" {
			return nil
		}
		list := []string{}
		if value != "-" {
			for _, v := range strings.Split(value, ",") {
				if v = strings.TrimSpace(v); v != "" {
					list = append(list, v)
				}
			}
		}
		if choice == "6" {
			c.BypassDomains = list
		} else {
			c.BypassCIDRs = list
		}
	case "8":
		value, err = a.read("Максимум одновременных подключений (1–1024):")
		if err != nil {
			return err
		}
		n, e := strconv.Atoi(value)
		if e != nil || n < 1 || n > 1024 {
			return fmt.Errorf("нужно число 1–1024")
		}
		c.MaxConnections = n
	case "9":
		value, err = a.read("Интервал запуска TCP-кандидатов в Smart без relay, ms (10–2000; обычно 250):")
		if err != nil {
			return err
		}
		n, e := strconv.Atoi(value)
		if e != nil {
			return e
		}
		c.DirectRaceDelayMS = n
	default:
		return fmt.Errorf("неизвестный пункт")
	}
	if err != nil {
		return err
	}
	if err = saveConfig(a.configPath, c); err != nil {
		return fmt.Errorf("настройки не изменены: %w", err)
	}
	a.print("\n  %s\n", a.tint("92", "Сохранено. Новые настройки применятся при запуске ядра."))
	return nil
}
