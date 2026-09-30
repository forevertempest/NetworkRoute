package console

import (
	"fmt"
	resources "networkroute"
	"strings"
)

func (a *App) guide() error {
	a.header("Встроенное руководство · доступно без интернета")
	a.print("  [1] Первый запуск и все пункты меню\n  [2] Подключение VPS relay\n  [3] Настройка своего WireGuard VPN\n  [4] Методика измерений\n  [5] Безопасность и ограничения\n  [6] Архив проверок релиза\n  [7] Лицензии сторонних компонентов\n  [8] Локальное управление, автозапуск и Linux\n  [0] Назад\n")
	choice, err := a.read("Раздел:")
	if err != nil {
		return err
	}
	paths := map[string]string{"1": "docs/CONSOLE.md", "2": "docs/QUICKSTART.md", "3": "tools/setup-wireguard.md", "4": "docs/BENCHMARKS.md", "5": "docs/SECURITY.md", "6": "docs/VALIDATION.md", "7": "docs/THIRD_PARTY_NOTICES.md"}
	paths["8"] = "docs/LOCAL_CONTROL.md"
	if choice == "0" || choice == "" {
		return nil
	}
	path, ok := paths[choice]
	if !ok {
		return fmt.Errorf("неизвестный раздел")
	}
	return a.document(path)
}

func wrap(s string, width int) []string {
	if s == "" {
		return []string{""}
	}
	r := []rune(s)
	result := []string{}
	for len(r) > width {
		cut := width
		for i := width; i > width/2; i-- {
			if r[i] == ' ' {
				cut = i
				break
			}
		}
		result = append(result, string(r[:cut]))
		r = r[cut:]
		for len(r) > 0 && r[0] == ' ' {
			r = r[1:]
		}
	}
	return append(result, string(r))
}

func (a *App) document(path string) error {
	data, err := resources.Files.ReadFile(path)
	if err != nil {
		return err
	}
	a.header("Руководство")
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		for _, part := range wrap(safe(line), 74) {
			if strings.HasPrefix(part, "#") {
				a.print("  %s\n", a.tint("96", strings.TrimLeft(part, "# ")))
			} else {
				a.print("  %s\n", part)
			}
			count++
			if count%18 == 0 {
				answer, e := a.read("Enter — следующая страница; 0 — закончить:")
				if e != nil {
					return e
				}
				if answer == "0" {
					return nil
				}
			}
		}
	}
	a.print("\n  Конец раздела.\n")
	return nil
}
