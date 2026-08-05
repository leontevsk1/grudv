# API vpn-core

Все эндпоинты, кроме `/api/sub/{tg_id}`, требуют заголовок `Authorization: Bearer <token>`.

- Эндпоинты бота (`/api/v1/users*`, `/api/v1/payments*`, `/api/v1/nodes*`) — токен это `BOT_SECRET`, общий секрет бот↔мастер.
- `/api/v1/nup/config` и `/api/v1/nup/node-keys` — токен это `join_token` конкретного узла, выдаётся при регистрации узла (`POST /api/v1/nodes`) и хранится в таблице `nodes`.

| Метод | Путь | Кто вызывает | Действие |
|---|---|---|---|
| POST | `/api/v1/users` | бот | создать/продлить пользователя (upsert по `tg_id`) |
| GET | `/api/v1/users/{tg_id}` | бот | получить тариф и креды пользователя |
| DELETE | `/api/v1/users/{tg_id}` | бот | удалить пользователя |
| POST | `/api/v1/payments/{id}/approve` | бот (от админа) | подтвердить оплату, продлить `expire_at`, включить `premium` |
| POST | `/api/v1/payments/{id}/reject` | бот (от админа) | отклонить заявку |
| POST | `/api/v1/nodes` | бот/админ | зарегистрировать узел, сгенерировать `join_token` |
| DELETE | `/api/v1/nodes/{id}` | бот/админ | удалить узел |
| GET | `/api/v1/nodes/{id}/config` | бот/админ | прочитать декларативный JSONB-конфиг узла (inbounds/outbounds/route) |
| PUT | `/api/v1/nodes/{id}/config` | бот/админ | задать/заменить декларативный JSONB-конфиг узла |
| GET | `/api/v1/nup/config` | `nup` (Bearer `join_token`) | pull актуального конфига для своего узла |
| POST | `/api/v1/nup/node-keys` | `nup` (Bearer `join_token`) | однократно при старте узла: сохранить публичный Reality-ключ + short_id узла в БД |
| GET | `/api/sub/{tg_id}` | клиент (без авторизации) | base64-список ссылок для VPN-клиента |

## Тела запросов

**`POST /api/v1/users`**
```json
{ "tg_id": 123456789, "tier": "premium", "add_days": 30 }
```
`add_days` необязателен (дефолт 30 при `tier: "premium"`). При `tier: "free"` — `expire_at` сбрасывается в `null`.

**`POST /api/v1/nodes`**
```json
{ "name": "reality-1", "address": "1.2.3.4", "node_type": "reality", "upstream_node_id": null }
```
`upstream_node_id` обязателен только для `node_type: "relay"` — id узла-мишени. Ответ содержит сгенерированный `join_token`.

**`POST /api/v1/nup/node-keys`**
```json
{ "public_key": "hmO35xA5fmKhPmZB-MwA_Hj7x0J9UDhr0Hpgp94fyzQ", "short_id": "da6a75a3245fdd29" }
```
Вызывается самим `nup` при первом запуске на `reality`/`relay`-узлах (`nup/reality.go::PushRealityPublicKey`) — не требует ручного вызова. Приватный ключ никогда не отправляется, только публичная часть.

**`GET /api/v1/nup/config`** — без тела, ответ:
```json
{
  "node": { "id": 1, "name": "reality-1", "address": "1.2.3.4", "node_type": "reality", "upstream_node_id": null, "join_token": "...", "reality_pub_key": "...", "reality_short_id": "...", "config": { "...": "см. PUT /api/v1/nodes/{id}/config" } },
  "users": [ { "tg_id": 123, "tier": "premium", "expire_at": "...", "vless_uuid": "...", "hy2_password": "...", "tuic_uuid": "...", "tuic_password": "..." } ],
  "upstream_node": null
}
```
`upstream_node` заполнен только для `relay`-узлов — это узел, на который релей проксирует трафик; содержит его `reality_pub_key`/`reality_short_id`, которые `relay` использует как клиент Reality.

Для `reality`/`web`-узлов без сохранённого `config` этот эндпоинт отвечает `409 Conflict` — узел зарегистрирован, но оператор ещё не задал ему протоколы через `PUT /api/v1/nodes/{id}/config`. `relay`-узлы не используют `config` — топология релея пока генерируется старым Go-кодом (`nup/template.go::buildRelayConfig`).

**`PUT /api/v1/nodes/{id}/config`** — декларативное описание топологии узла (inbounds/outbounds/route), интерпретируемое `nup` при следующем pull'е. Секреты (Reality private key, TLS-сертификаты) сюда не входят — их подставляет `nup` локально на воркере.
```json
{
  "version": 1,
  "log_level": "info",
  "inbounds": [
    {
      "tag": "in-vless-reality",
      "type": "vless",
      "listen": "::",
      "listen_port": 443,
      "protocol_settings": { "flow": "xtls-rprx-vision" },
      "transport": null,
      "tls": { "mode": "reality", "server_name": "telemetry.mozilla.org" },
      "user_ids": [123456789, 987654321],
      "credential_field": "vless"
    }
  ],
  "outbounds": [
    { "tag": "Direct-Premium", "type": "direct", "routing_mark": null },
    { "tag": "Direct-Free", "type": "direct", "routing_mark": 100 },
    { "tag": "Block", "type": "block" }
  ],
  "route": {
    "rules": [
      { "ip_is_private": true, "action": "route", "outbound": "Block" },
      { "auth_user": ["premium"], "action": "route", "outbound": "Direct-Premium" },
      { "auth_user": ["free"], "action": "route", "outbound": "Direct-Free" }
    ],
    "final": "Block"
  }
}
```
- `tls.mode` — `"cert"` (сертификат Caddy/certbot, подставляется по фиксированному пути) или `"reality"` (приватный ключ узла, никогда не хранится в БД).
- `credential_field` — `"vless"` / `"hy2"` / `"tuic"` / `"naive"`, какое поле пользователя проецировать в inbound.
- `user_ids` — список `tg_id`, подключённых к этому inbound.
- `routing_mark`/tc-throttling по-прежнему определяется `users.tier` глобально, не этой моделью.

Ответ `422 Unprocessable Entity` — при опечатке в структуре (serde) или ссылке на несуществующий `tag` в `route.rules`/`route.final`.
