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

Собирает `nup` под `linux/amd64` и `linux/arm64` (`CGO_ENABLED=0`, статически, `-trimpath -ldflags "-s -w"`), публикует бинарники как GitHub Release assets (`nup-linux-amd64`, `nup-linux-arm64`). `personal/worker-bootstrap.sh` скачивает их через `gh release download --repo leontevsk1/grudv --pattern nup-linux-${ARCH}`.

## Чего в CI нет (делается вручную)

- **Деплоя на сервер из CI нет.** GHCR получает свежий образ на каждый push в `main`, но накатывание на реальный сервер — отдельный ручной шаг через `personal/deploy-master.sh`/`deploy-worker.sh` (первый запуск) или `personal/render-env.sh` + `docker/podman compose pull && up -d` (обновление уже работающего сервера). Подробности — `personal/worker_manual.md` (не в git).
- **Smoke-test после деплоя нет** — работоспособность после обновления проверяется вручную (`journalctl -u nup.service`, `curl` по API).
- **Разделения staging/production нет** — один мастер-сервер, деплой туда же, куда и разработка целится.

Обоснование, почему это не автоматизировано прямо сейчас: один сервер на роль, YAGNI — вводить полноценный CD-пайплайн стоит, когда ручной процесс станет болью (несколько серверов, частые релизы), не раньше.
