# CI/CD — что реально есть

## CI (`.github/workflows/ci.yml`)

Триггеры: любой pull request, и push в `main`.

- **rust-build** — `cargo build --release` + `cargo clippy -- -D warnings` в `vpn-core/`, с `SQLX_OFFLINE=true` (нет живой БД в раннере — используется закоммиченный `.sqlx/` query-кэш).
- **go-build** — матрица `[bot, nup]`: `go build ./...` + `go vet ./...` в каждом модуле.
- **rust-test** — `cargo test` в `vpn-core/` (зависит от rust-build).
- **go-test** — матрица `[bot, nup]`: `go test ./...` (зависит от go-build).
- **build-and-push** — только на push в `main`: собирает Docker-образы `vpn-core` и `bot`, пушит в GHCR (`ghcr.io/leontevsk1/grudv/vpn-core:<sha>`, `ghcr.io/leontevsk1/grudv/bot:<sha>`), тег — полный git sha коммита.

`nup` в этот пайплайн не входит — он не контейнеризуется (systemd-бинарник на голом хосте воркера, см. `docs/dev/architecture.md`), только собирается/тестируется.

## Release (`.github/workflows/release.yml`)

Триггер: push тега `v*`.

Собирает `nup` под `linux/amd64` и `linux/arm64` (`CGO_ENABLED=0`, статически, `-trimpath -ldflags "-s -w -X main.version=<тег>"`), публикует бинарники как GitHub Release assets (`nup-linux-amd64`, `nup-linux-arm64`). `personal/worker-bootstrap.sh` скачивает их через `gh release download --repo leontevsk1/grudv --pattern nup-linux-${ARCH}` при первичной установке узла; после установки `nup` следит за новыми релизами сам (см. ниже).

## Health-check (`GET /health` в vpn-core)

Без авторизации, делает `SELECT 1` через пул: `200` при рабочей БД, `503` при недоступной (процесс не падает). Не завязан на схему — обычный `sqlx::query`, не макрос, поэтому не требует `cargo sqlx prepare` при изменении. Существует для внешних smoke-тестов (после деплоя, из мониторинга) — сам по себе ничего не автоматизирует.

## Self-update `nup` (`nup/selfupdate.go`)

`nup` — pull-only edge-нода (см. `docs/dev/architecture.md`), поэтому обновление тоже инициирует сам демон, а не мастер или CI: отдельная горутина с тикером в 12 часов (`runSelfUpdateLoop` в `nup/main.go`, не завязана на минутный конфиг-цикл) опрашивает `GET https://api.github.com/repos/leontevsk1/grudv/releases/latest` с тем же `GH_TOKEN`, что уже используется в `worker-bootstrap.sh` для `gh release download`. Если тег релиза новее версии, вшитой в бинарник через `-X main.version`, скачивает нужный ассет (`nup-linux-amd64`/`nup-linux-arm64`), атомарно подменяет исполняемый файл (`os.Rename` на той же ФС) и вызывает `systemctl restart nup.service` — без нового sudoers-правила, `nup.service` уже работает от root (нет `User=` в unit-файле).

Локальные сборки (`version = "dev"`, дефолт без ldflags) самообновление пропускают — иначе разработческий бинарник начал бы подменять себя релизным.

## SSH-деплой `vpn-core`/`bot` (`deploy-master`/`deploy-bot` в `ci.yml`)

Оба джоба идут после `build-and-push`, только на push в `main`. Правят точечно `IMAGE_TAG` в уже существующем `.env` на сервере (`sed -i`) и перезапускают один сервис через `podman-compose pull && up -d` — секреты (`POSTGRES_PASSWORD`/`BOT_SECRET`/токен бота) через CI не передаются вообще, они остаются там, куда их положил `sops` при первичном bootstrap (`personal/deploy-master.sh`/`personal/deploy-bot.sh`).

Master и bot — обычные VPS под полным контролем оператора (не edge-ноды воркеров), но контейнерный движок везде один — rootless `podman`/`podman-compose` под выделенным `deploy`-пользователем (`loginctl enable-linger deploy`), тот же выбор, что и на воркерах (`nup/config.go::detectContainerEngine`, подман в приоритете). Явные `container_name` в каждом `compose.yaml` (`vpn-core`, `vpn-postgres`, `bot`) — чтобы CI мог адресовать конкретный контейнер напрямую (`podman-compose up -d <service>`, `podman inspect <container_name>`), не полагаясь на автогенерируемое имя.

- **`deploy-master`** — обновляет `vpn-core` на мастере, затем smoke-test через `GET /health` (см. выше) — отличает "контейнер поднялся" от "поднялся, но не видит Postgres".
- **`deploy-bot`** — обновляет `bot` на отдельном VPS. У бота нет HTTP-эндпоинта (long-polling клиент Telegram), поэтому проверка — `podman inspect bot --format '{{.State.Status}}'`, не curl.
- Один `DEPLOY_SSH_KEY` на оба сервера (не два) — при текущей модели угроз (утечка из GitHub Secrets) число ключей не меняет исход компрометации, один ключ проще поддерживать операционно. Хосты/юзеры (`MASTER_SSH_HOST`/`MASTER_SSH_USER`, `BOT_SSH_HOST`/`BOT_SSH_USER`) — раздельные секреты, чтобы джобы не путали серверы.

**Секретов в GitHub Actions сейчас нет** — джобы существуют в коде, но не запустятся содержательно, пока оператор не заведёт `DEPLOY_SSH_KEY`, `MASTER_SSH_HOST`, `MASTER_SSH_USER`, `BOT_SSH_HOST`, `BOT_SSH_USER` (см. `personal/future_improvement.md` → "Инструкция для оператора: настройка SSH-ключа для CI").

Первичная подготовка нового bot-VPS — `personal/bot-bootstrap.sh` (по аналогии с `master-bootstrap.sh`, но без Postgres/Caddy — бот не принимает inbound HTTP) + `personal/deploy-bot.sh` как обёртка через `sops`.

## Чего в CI нет (делается вручную)

- **Регистрация нового физического узла/сервера не автоматизирована** — это первичный bootstrap ещё не настроенного сервера (SSH туда, где заранее ничего нет), тот же паттерн ручного one-off, что и раньше. Автоматизируется в CD только обновление уже существующих серверов.
- **Rollback при упавшем smoke-test** — red build + ручной откат оператором, автоматического отката нет.
- **Разделения staging/production нет** — один мастер-сервер, деплой туда же, куда и разработка целится.

Обоснование, почему это не автоматизировано прямо сейчас: один сервер на роль, YAGNI — вводить полноценный CD-пайплайн стоит, когда ручной процесс станет болью (несколько серверов, частые релизы), не раньше.
