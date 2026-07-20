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
  "node": { "id": 1, "name": "reality-1", "address": "1.2.3.4", "node_type": "reality", "upstream_node_id": null, "join_token": "...", "reality_pub_key": "...", "reality_short_id": "..." },
  "users": [ { "tg_id": 123, "tier": "premium", "expire_at": "...", "vless_uuid": "...", "hy2_password": "...", "tuic_uuid": "...", "tuic_password": "..." } ],
  "upstream_node": null
}
```
`upstream_node` заполнен только для `relay`-узлов — это узел, на который релей проксирует трафик; содержит его `reality_pub_key`/`reality_short_id`, которые `relay` использует как клиент Reality.
