# Собственный WireGuard VPN: генерация и установка

Это необязательный помощник для стандартного WireGuard, отдельный от QUIC relay. Он генерирует совместимые профили; пакеты передаёт официальный WireGuard. NetworkRoute не устанавливает свой VPN-драйвер. ПО бесплатно; Linux-узел, его подключение и доступность предоставляет пользователь.

Первая версия помощника поддерживает **один Windows-клиент на один серверный профиль**, фиксированную сеть `10.203.0.0/24`, Linux с systemd и два режима:

| Режим | Результат |
|---|---|
| Split, по умолчанию | VPN только до `10.203.0.1` на VPS. Default route и DNS Windows сохраняются. Подходит для проверки VPN; интернет через VPS не направляется. |
| Full, явный выбор | IPv4-интернет через VPS, явный DNS, default routes `/0`. IPv6-интернет этой конфигурацией не предоставляется и вне VPN блокируется. |

Full-профиль использует поведение kill switch официального клиента Windows; при недоступном сервере интернет блокируется до отключения туннеля. Это отличается от fail-open прокси NetworkRoute. Одновременно включённые full-tunnel VPN могут конфликтовать; помощник не отключает чужой VPN. Перед активацией также проверьте отсутствие сети `10.203.0.0/24` в LAN/другом VPN Windows; Linux-установщик проверяет пересечения серверных маршрутов. См. [официальное описание маршрутов и firewall WireGuard Windows](https://git.zx2c4.com/wireguard-windows/about/docs/netquirk.md).

## 1. Windows: создать ключ локально

Установите [официальный WireGuard для Windows 10/11](https://www.wireguard.com/install/). Из корня распакованного Windows-архива NetworkRoute выполните:

```powershell
.\bin\networkroute-windows-amd64.exe vpn-prepare --directory state/vpn
```

Передайте **только** `state/vpn/client-wireguard.json` на VPS по SSH/SCP. Приватный ключ остаётся в Windows. На ARM64 используйте соответствующий бинарник.

## 2. VPS: создать серверный профиль

На Debian/Ubuntu установите зависимости:

```bash
sudo apt-get update
sudo apt-get install -y wireguard nftables iproute2 python3
```

Нужны поддержка WireGuard ядром и права создания интерфейса; контейнер без таких прав не подходит. Из распакованного Linux-архива NetworkRoute:

```bash
chmod +x bin/nr-relay-linux-amd64
./bin/nr-relay-linux-amd64 vpn-setup --directory vpn-bundle --endpoint vpn.example.com:51820 --client-request ~/client-wireguard.json
```

Замените `vpn.example.com` своим адресом. Это split-профиль. Для интернета через VPN **вместо предыдущего вызова** используйте full-вариант, подставив своё имя uplink-интерфейса и выбранный DNS:

```bash
ip -4 route show default
./bin/nr-relay-linux-amd64 vpn-setup --directory vpn-bundle --endpoint vpn.example.com:51820 --client-request ~/client-wireguard.json --full-tunnel --egress eth0 --dns 1.1.1.1
```

`eth0` — пример, часто используется `ens3` или другое имя. `1.1.1.1` — явный пример DNS; генератор сам DNS-провайдера не выбирает. Если уже создан другой профиль, используйте новый каталог: помощник не перезаписывает отличающиеся файлы или ключи.

## 3. VPS: установить и проверить

```bash
bash tools/install-wireguard.sh --bundle vpn-bundle
sudo bash tools/install-wireguard.sh --bundle vpn-bundle --apply
sudo systemctl status wg-quick@nro-wg
```

Без `--apply` выводится только план. Установщик сохраняет `/etc/wireguard/nro-wg.conf`, запускает `wg-quick@nro-wg`, проверяет наличие зависимостей, конфликты адресов и своих путей. Full-режим включает IPv4 forwarding и собственную nftables NAT-таблицу `ip networkroute_wg`; чужие правила не удаляются.

Разрешите входящий UDP 51820 в firewall VPS и cloud firewall. Для full-режима firewall должен также разрешать forwarding от `nro-wg` с адресом `10.203.0.2` к uplink и ответный established/related трафик. NAT **не отменяет** существующие FORWARD drop-правила. В средах с UFW/firewalld используйте правила своего firewall; установщик не заменяет его политику. Дополнительная маршрутизация у VPS-провайдера может требовать настройки в его панели.

## 4. Windows: импортировать

Заберите **только** `vpn-bundle/wireguard-connection.json` с VPS по доверенному SSH-каналу. Он содержит публичные ключи и настройки, но не приватный ключ сервера. Выполните:

```powershell
.\bin\networkroute-windows-amd64.exe vpn-pair --directory state/vpn --ticket wireguard-connection.json
```

Для full-профиля требуется явное добавление `--allow-full-tunnel`. Затем в официальном клиенте WireGuard: **Import tunnel(s) from file → `state/vpn/nro-client.conf` → Activate**. Генерация профиля сама ничего не активирует. `nro-client.conf` содержит приватный ключ: не отправляйте его на GitHub и другим пользователям.

## 5. Проверить фактическую работу

На Windows: `ping 10.203.0.1` (ответ зависит также от firewall). На VPS: `sudo wg show nro-wg` — после активации должны обновиться latest handshake и счётчики передачи. Одного статуса «Active» в интерфейсе Windows недостаточно.

Для full-профиля отдельно проверьте DNS (`Resolve-DnsName example.com`) и HTTPS (`curl.exe https://example.com/`). Если handshake есть, но интернет не работает, проверьте forwarding, uplink, NAT и FORWARD firewall; если handshake нет — endpoint, UDP-порт, ключи и облачный firewall. IPv6-интернет здесь ожидаемо недоступен.

Чтобы использовать NetworkRoute поверх этого VPN, выберите `setup --mode vpn`, затем `doctor` и `serve`, как в [быстром запуске](../docs/QUICKSTART.md). WireGuard сам направляет системный трафик согласно профилю; адаптивный выбор Direct/Relay для всей системы этот помощник не добавляет. QUIC relay и WireGuard не обязательно запускать одновременно.

## Остановка и ограничения

Windows: Deactivate в WireGuard. Linux: `sudo systemctl disable --now wg-quick@nro-wg`. При штатной остановке full-профиля его NAT-таблица удаляется, файлы ключей сохраняются. Глобальный IPv4 forwarding остаётся включён: автоматическое отключение могло бы сломать другой VPN/маршрутизатор. Администратор может восстановить прежнее значение, если оно больше никому не нужно. После сбоя запуска проверьте journal и наличие таблицы `networkroute_wg` перед повторным запуском; автоматический crash recovery этого установщика не заявляется.

MTU вручную не задаётся: его определяют стандартные WireGuard-клиенты. Hooks выполняет Linux `wg-quick`, поэтому устанавливайте только собственный сгенерированный и проверенный bundle. Формат и поведение: [wg-quick(8)](https://git.zx2c4.com/wireguard-tools/about/src/man/wg-quick.8). Генерация и валидация профилей тестируются автоматически; полная проверка внешнего VPN требует реального Linux-узла и Windows-клиента.
