// Package console provides a persistent, dependency-free interactive front end.
package console

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"networkroute/internal/config"
	"networkroute/internal/engine"
	"networkroute/internal/localnet"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type App struct {
	ctx                      context.Context
	dir, version, configPath string
	out                      io.Writer
	lines                    chan string
	readErr                  chan error
	color                    bool
	session                  *engine.Session
	logs                     logBuffer
	diagnostics              func(string) error
	StartImmediately         bool
}

func Launch(ctx context.Context, dir, version string, diagnostics func(string) error, start ...bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if dir == "" {
		var err error
		dir, err = DefaultDirectory()
		if err != nil {
			return err
		}
	}
	ansi, restore := terminal()
	defer restore()
	a := New(ctx, dir, version, os.Stdin, os.Stdout)
	a.StartImmediately = len(start) > 0 && start[0]
	a.color, a.diagnostics = ansi, diagnostics
	return a.Run()
}

func New(ctx context.Context, dir, version string, input io.Reader, out io.Writer) *App {
	a := &App{ctx: ctx, dir: dir, version: version, out: out, lines: make(chan string), readErr: make(chan error, 1)}
	go func() {
		scanner := bufio.NewScanner(input)
		scanner.Buffer(make([]byte, 4096), 65536)
		for scanner.Scan() {
			select {
			case a.lines <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
		err := scanner.Err()
		if err == nil {
			err = io.EOF
		}
		a.readErr <- err
	}()
	return a
}

func (a *App) print(format string, args ...any) { fmt.Fprintf(a.out, format, args...) }
func (a *App) tint(code, text string) string {
	if a.color {
		return "\x1b[" + code + "m" + text + "\x1b[0m"
	}
	return text
}
func (a *App) read(prompt string) (string, error) {
	a.print("\n  %s ", prompt)
	select {
	case line := <-a.lines:
		return strings.TrimSpace(line), nil
	case err := <-a.readErr:
		return "", err
	case <-a.ctx.Done():
		return "", a.ctx.Err()
	}
}
func (a *App) pause() error {
	_, err := a.read("Enter — вернуться в меню:")
	return err
}
func (a *App) failure(err error) {
	a.print("\n  %s %s\n", a.tint("91", "Ошибка:"), safe(err.Error()))
}
func (a *App) header(title string) {
	if a.color {
		a.print("\x1b[2J\x1b[H")
	}
	a.print("\n%s\n", a.tint("96", "  ╔══════════════════════════════════════════════════════════════════════╗"))
	a.print("%s\n", a.tint("96", "  ║  N E T W O R K R O U T E                         CONNECTION CONTROL  ║"))
	a.print("%s\n", a.tint("96", "  ╚══════════════════════════════════════════════════════════════════════╝"))
	a.print("  %s  ·  %s\n\n", a.tint("1", safe(title)), safe(a.version))
}

func (a *App) Run() error {
	if err := os.MkdirAll(a.dir, 0700); err != nil {
		return err
	}
	unlock, err := lockDirectory(a.dir)
	if err != nil {
		a.failure(fmt.Errorf("не удалось занять каталог данных; возможно, NetworkRoute уже запущен: %w", err))
		a.pause()
		return err
	}
	defer unlock()
	if err := a.loadState(); err != nil {
		a.header("Не удалось открыть настройки")
		a.failure(err)
		a.print("  Данные: %s\n  Можно выбрать другую папку: NetworkRoute.exe menu --data-dir PATH\n", safe(a.dir))
		a.pause()
		return err
	}
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&a.logs, nil)))
	defer slog.SetDefault(oldLogger)
	defer a.stop()
	local, err := localnet.Load(a.dir)
	if err != nil {
		a.failure(err)
		a.pause()
		return err
	}
	if a.StartImmediately || local.StartOnLaunch {
		if err := a.toggle(); err != nil {
			a.failure(err)
			if e := a.pause(); e != nil {
				return e
			}
		}
	}
	for {
		a.header("Панель управления")
		if a.session != nil {
			select {
			case <-a.session.Done:
				if err := a.session.Err(); err != nil {
					a.failure(err)
				}
				a.session = nil
			default:
			}
		}
		c, err := config.Load(a.configPath)
		if err != nil {
			a.failure(err)
		} else {
			state := a.tint("90", "ОСТАНОВЛЕН")
			if a.session != nil {
				state = a.tint("92", "РАБОТАЕТ")
			}
			a.print("  Состояние  %s    Режим %s / %s\n", state, safe(c.Mode), safe(string(c.Profile)))
			a.print("  SOCKS5     %s    Fail-%s\n", safe(c.Listen), safe(c.FailMode))
			relay := c.Relay.Address
			if relay == "" {
				relay = "не требуется · автономный режим"
			}
			a.print("  Relay      %s\n", safe(relay))
			if a.session != nil {
				a.print("  Потоки     %d активных · передано %.2f MiB\n", a.session.Router.Metrics.Active.Load(), float64(a.session.Router.Metrics.Bytes.Load())/1048576)
			}
		}
		a.print("\n  %s\n", a.tint("90", "Обрабатываются приложения с настроенным SOCKS5. Системный VPN — отдельно."))
		a.print("\n  [1] Запустить / остановить      [2] Настройки подключения\n  [3] VPN и собственный VPS       [4] Сеть и приложения\n  [5] Сравнить качество маршрута  [6] Проверить конфигурацию\n  [7] Руководство и первый запуск [8] Счётчики и журнал\n  [9] Расширенная диагностика    [10] Локальное управление\n [11] Автозапуск и запуск ядра    [0] Выход\n\n  Настройки: %s\n", safe(a.configPath))
		choice, err := a.read("Выберите пункт (Enter — обновить):")
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		if choice == "" {
			continue
		}
		if choice == "0" {
			a.print("\n  Завершение. Если использовали прокси — уберите его в приложении.\n")
			return nil
		}
		switch choice {
		case "1":
			err = a.toggle()
		case "2":
			err = a.settings()
		case "3":
			err = a.connections()
		case "4":
			err = a.network()
		case "5":
			err = a.benchmark()
		case "6":
			err = a.doctor()
		case "7":
			err = a.guide()
		case "8":
			err = a.statistics()
		case "9":
			a.header("Диагностика сети")
			if a.diagnostics != nil {
				err = a.diagnostics("diagnostics")
			} else {
				err = fmt.Errorf("расширенная диагностика доступна в Windows CLI")
			}
		case "10":
			err = a.localControls()
		case "11":
			err = a.startupSettings()
		default:
			a.print("\n  Используйте номера 0–11.\n")
		}
		if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
			return nil
		}
		if err != nil {
			a.failure(err)
		}
		if err = a.pause(); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
	}
}

func (a *App) stop() error {
	if a.session == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := a.session.Stop(ctx)
	if err == nil {
		a.session = nil
	}
	return err
}
func (a *App) toggle() error {
	if a.session != nil {
		if err := a.stop(); err != nil {
			return err
		}
		a.print("\n  Прокси остановлен. Подключения закрыты. Уберите SOCKS5 в приложении.\n")
		return nil
	}
	c, err := config.Load(a.configPath)
	if err != nil {
		return err
	}
	s, err := engine.Start(a.ctx, c)
	if err != nil {
		return err
	}
	a.session = s
	a.print("\n  %s\n  Адрес SOCKS5: %s\n  Включите этот прокси в нужном приложении.\n  Можно пользоваться меню, пока ядро работает. Закрытие окна остановит ядро.\n", a.tint("92", "Сетевое ядро запущено."), safe(s.Address))
	return nil
}
func (a *App) requireStopped() error {
	if a.session != nil {
		return fmt.Errorf("сначала остановите ядро пунктом 1; изменение настроек не должно обрывать текущие соединения неожиданно")
	}
	return nil
}
func (a *App) dataPath(name string) string { return filepath.Join(a.dir, name) }
