# grudv

Самодельная система управления VPN-инфраструктурой. Не панель типа Marzban или 3X-UI — своя минималистичная замена, потому что панели тяжело автоматизировать и тяжело менять, когда нужно что-то нестандартное.

Три независимых процесса — **vpn-core** (Rust, единственный stateful компонент), **bot** (Go, телеграм-бот) и **nup** (Go, демон на каждом воркере, pull-модель) — плюс PostgreSQL как источник истины. Подробная концепция, как компоненты работают вместе, владение Reality-ключами и типы воркеров — [`docs/dev/architecture.md`](docs/dev/architecture.md). Вся остальная документация разработчика — [`docs/dev/`](docs/dev/README.md).

## Монетизация

Freemium. У бесплатных пользователей трафик в sing-box идёт через отдельный outbound с `routing_mark`, и `nup` на основании этого маркера накладывает `tc`-лимит (сейчас — жёсткий потолок в 512 Кбит/с). Достаточно, чтобы не смотреть видео, но чтобы зайти в телеграм — хватает.

Платежи не через API банка или ЮKassa — всё вручную: пользователь пишет боту "Я оплатил", админ смотрит на реальный перевод и жмёт кнопку. Никакой автоматической сверки, зато не нужно возиться с эквайрингом.

## API vpn-core

Полная таблица эндпоинтов с телами запросов/ответов — [`docs/dev/api.md`](docs/dev/api.md).

## Запуск локально

`personal/start-services.sh` был написан для запуска из корня репозитория (там лежат `vpn-core/`, `bot/`, `nup/`, `compose.yaml`, `.env`), но сейчас физически лежит в `personal/`, а внутри всё ещё резолвит пути от собственной директории — **на текущем месте не работает**. Дожидается своей починки; пока что сервисы (`vpn-core`, `bot`, `nup`) поднимаются вручную по отдельности, `compose.yaml` в корне — для Postgres и тестового sing-box.

Нужен `.env` в корне (переменные: `DATABASE_URL`, `TELEGRAM_BOT_TOKEN`, `ADMIN_TG_ID`, `MASTER_URL`, `BOT_SECRET`, `NODE_JOIN_TOKEN`).

## Секреты

`.env`-файлы — только для локального запуска, в git не попадают (`.gitignore`). Источник правды для секретов — `secrets.enc.yaml` в корне, зашифрованный через [sops](https://github.com/getsops/sops) + [age](https://github.com/FiloSottile/age); коммитится в открытом виде, расшифровать может только тот, у кого есть приватный age-ключ (`~/.config/sops/age/keys.txt`, не в репозитории).

```bash
sops secrets.enc.yaml              # открыть на редактирование ($EDITOR), сохранение — авто-перешифровка
sops --decrypt secrets.enc.yaml    # посмотреть всё в открытом виде
sops --decrypt --extract '["bot_secret"]' secrets.enc.yaml   # одно поле
```

Деплой-скрипты (`personal/deploy-worker.sh`, `personal/deploy-master.sh`) читают секреты отсюда сами — переменные вроде `BOT_SECRET` больше не набираются руками в SSH-команде (это и было источником прошлой опечатки). `.sops.yaml` в корне задаёт, каким age-ключом шифровать; при добавлении второго человека к проекту — добавить его публичный age-ключ туда и запустить `sops updatekeys secrets.enc.yaml`.

### `.env` для ручного обновления уже работающего сервера

`deploy-worker.sh`/`deploy-master.sh` — только для первого запуска (bootstrap с нуля). Когда сервис уже крутится (например, бот на отдельном VPS через `compose.yaml`) и нужно просто обновить `.env` — `personal/render-env.sh` рендерит его из `secrets.enc.yaml` + non-secret параметров, сам файл никогда не лежит на диске дольше, чем до `scp`:

```bash
./personal/render-env.sh bot https://<MASTER_HOST> <IMAGE_TAG> > /tmp/bot.env
scp /tmp/bot.env user@rvhost:~/bot-stack/.env && rm /tmp/bot.env

./personal/render-env.sh master <IMAGE_TAG> > /tmp/master.env
scp /tmp/master.env deploy@<MASTER_HOST>:/opt/vpn/.env && rm /tmp/master.env

./personal/render-env.sh nup <MASTER_URL> <NODE_JOIN_TOKEN> <NODE_TYPE> <DOMAIN> > /tmp/nup.env
scp /tmp/nup.env root@<WORKER_IP>:/etc/nup/config.env && rm /tmp/nup.env
```

## Состояние проекта

MVP. CI собирает и тестирует все три компонента на каждый PR/push в `main`, `vpn-core`/`bot` пушатся в GHCR как Docker-образы, `nup` релизится как бинарник по git-тегу — подробнее в [`docs/dev/ci-cd.md`](docs/dev/ci-cd.md). Первый запуск сервера и обновление уже работающего — вручную через `personal/` (см. `docs/dev/ci-cd.md` → "Чего в CI нет"). Известные проблемы — [`docs/dev/known-issues.md`](docs/dev/known-issues.md).
