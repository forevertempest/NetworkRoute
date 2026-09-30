package console

import (
	"fmt"
	resources "networkroute"
	"networkroute/internal/onboarding"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) connections() error {
	if err := a.requireStopped(); err != nil {
		return err
	}
	a.header("VPN и VPS")
	a.print("  [1] Перейти в автономный режим / существующий VPN\n  [2] VPS relay: создать публичный запрос\n  [3] VPS relay: импортировать ответ сервера\n  [4] Экспортировать встроенный Linux-пакет для VPS\n  [5] WireGuard: создать ключ и публичный запрос\n  [6] WireGuard: импортировать ответ и создать профиль\n  [7] Открыть инструкцию VPS\n  [8] Открыть инструкцию WireGuard\n  [0] Назад\n\n  Приватные ключи остаются на своей стороне. ПО бесплатно.\n  VPS предоставляет пользователь; серверная инфраструктура не включена.\n")
	choice, err := a.read("Пункт:")
	if err != nil {
		return err
	}
	switch choice {
	case "0", "":
		return nil
	case "1":
		path := filepath.Join(a.dir, "profiles", "default", "config.local.json")
		if _, err = os.Stat(path); os.IsNotExist(err) {
			path, err = onboarding.SetupClient(filepath.Dir(path), "standalone")
		}
		if err != nil {
			return err
		}
		if err = a.activate(path); err != nil {
			return err
		}
		a.print("\n  Выбран автономный профиль: используется текущий маршрут Windows/VPN.\n")
	case "2":
		path, e := onboarding.SetupClient(a.dataPath("profiles/relay"), "relay")
		if e != nil {
			return e
		}
		a.print("\n  Передайте на VPS только этот файл по SSH/SCP:\n  %s\n\n  Экспортируйте Linux-пакет пунктом 4. Следуйте встроенному руководству VPS.\n  Вернувшийся relay-connection.json импортируйте пунктом 3.\n", safe(path))
	case "3":
		ticket, e := a.read("Полный путь к relay-connection.json (можно перетащить файл):")
		if e != nil {
			return e
		}
		path, e := onboarding.Pair(a.dataPath("profiles/relay"), strings.Trim(ticket, "\""), "respect")
		if e != nil {
			return e
		}
		if e = a.activate(path); e != nil {
			return e
		}
		a.print("\n  Relay привязан. Выполните «Проверить конфигурацию», затем сравните маршрут.\n  При обнаружении VPN relay по умолчанию приостанавливается.\n")
	case "4":
		b, e := resources.Files.ReadFile(".bundle/linux-relay.zip")
		if e != nil {
			return fmt.Errorf("в этой сборке нет Linux-пакета; используйте официальный release build")
		}
		dir, e := a.read("Папка экспорта (Enter — каталог данных):")
		if e != nil {
			return e
		}
		if dir == "" {
			dir = a.dir
		}
		dir = strings.Trim(dir, "\"")
		if e = os.MkdirAll(dir, 0700); e != nil {
			return e
		}
		path := filepath.Join(dir, "NetworkRoute-Linux-Relay.zip")
		f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write(b)
		closeErr := f.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		a.print("\n  Экспортирован пакет Linux amd64/arm64, установщики и руководства:\n  %s\n  Передайте архив на VPS по SSH/SCP. Windows EXE на Linux не запускается.\n", safe(path))
	case "5":
		path, e := onboarding.PrepareVPN(a.dataPath("vpn"))
		if e != nil {
			return e
		}
		a.print("\n  Передайте только публичный запрос на VPS:\n  %s\n  Для настройки сервера используйте пункт 8 и Linux-пакет (пункт 4).\n", safe(path))
	case "6":
		ticket, e := a.read("Полный путь к wireguard-connection.json:")
		if e != nil {
			return e
		}
		ticket = strings.Trim(ticket, "\"")
		var info onboarding.WGTicket
		if e = onboarding.ReadJSON(ticket, &info); e != nil {
			return e
		}
		allow := false
		if info.FullTunnel {
			a.print("\n  Этот full-профиль направляет IPv4-интернет через VPS.\n  IPv6-интернет блокируется. VPN меняет DNS и маршруты после активации.\n  При отказе VPN интернет блокируется до отключения туннеля.\n")
			answer, e := a.read("Создать такой профиль? Введите да:")
			if e != nil {
				return e
			}
			if answer != "да" {
				return nil
			}
			allow = true
		}
		path, e := onboarding.PairVPN(a.dataPath("vpn"), ticket, allow)
		if e != nil {
			return e
		}
		a.print("\n  Профиль: %s\n  В официальном WireGuard: Import tunnel(s) from file → этот файл → Activate.\n  https://www.wireguard.com/install/\n  Файл содержит приватный ключ: не публикуйте его.\n", safe(path))
	case "7":
		return a.document("docs/QUICKSTART.md")
	case "8":
		return a.document("tools/setup-wireguard.md")
	default:
		return fmt.Errorf("неизвестный пункт")
	}
	return nil
}
