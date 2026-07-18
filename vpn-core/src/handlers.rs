use std::net::SocketAddr;

use axum::{
    Json,
    extract::{ConnectInfo, Path, State},
    http::{HeaderMap, StatusCode},
    response::IntoResponse,
};
use base64::Engine;
use chrono::{Duration, Utc};
use sqlx::PgPool;
use uuid::Uuid;

use crate::{
    db,
    models::{NodeCreateRequest, NupConfigResponse, TrafficReport, UserUpsertRequest},
};

pub const SUB_IP_LIMIT: i64 = 20;
pub const PREMIUM_MONTHLY_LIMIT_BYTES: i64 = 200_000_000_000;

const SHARING_BLOCKED_MESSAGE: &str =
    "Ваша подписка была заблокирована из-за подозрения в совместном использовании. Обратитесь в поддержку.";
const LIMIT_REACHED_MESSAGE: &str =
    "Вы израсходовали месячный лимит 200 ГБ. Скорость снижена до 512 кбит/с до конца месяца.";

#[derive(Clone)]
pub struct AppState {
    pub db: PgPool,
    pub bot_secret: String,
}

// Вспомогательная функция для извлечения Bearer-токена
pub fn extract_bearer(headers: &HeaderMap) -> Option<String> {
    headers
        .get("authorization")
        .and_then(|v| v.to_str().ok())
        .and_then(|s| s.strip_prefix("Bearer ").map(String::from))
}

// Строит список ссылок для одного узла под конкретного пользователя.
// Троттлинг free-пользователей идёт на сервере по UUID (см. nup/template.go),
// а не по параметрам ссылки — клиент не может повлиять на свою полосу.
pub fn build_sub_configs(user: &crate::models::User, node_address: &str) -> Vec<String> {
    let mut configs = vec![format!(
        "vless://{}@{}?encryption=none&security=tls&type=httpupgrade",
        user.vless_uuid.unwrap_or_default(),
        node_address
    )];

    if user.tier != "free" {
        if let Some(hy2_pass) = &user.hy2_password {
            configs.push(format!("hy2://{}@{}", hy2_pass, node_address));
        }

        if let Some(tuic_uuid) = user.tuic_uuid {
            configs.push(format!("tuic://{}@{}", tuic_uuid, node_address));
        }
    }

    configs
}

// Premium-юзер, выбравший месячный лимит, до конца месяца обслуживается как free.
pub fn effective_tier(tier: &str, over_limit: bool) -> &str {
    if tier == "premium" && over_limit {
        "free"
    } else {
        tier
    }
}

// -----------------------------------------------------------------
// УПРАВЛЕНИЕ ПОЛЬЗОВАТЕЛЯМИ (API для Бота)
// -----------------------------------------------------------------

pub async fn upsert_user(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(payload): Json<UserUpsertRequest>,
) -> impl IntoResponse {
    let token = extract_bearer(&headers).unwrap_or_default();
    if token != state.bot_secret {
        return StatusCode::UNAUTHORIZED.into_response();
    }

    let expire_at = if payload.tier == "premium" {
        let days = payload.add_days.unwrap_or(30);
        Some((Utc::now() + Duration::days(days)).naive_utc())
    } else {
        None
    };

    // Генерируем полный набор ключей сразу.
    // При конфликте db::upsert_user обновит только tier и expire_at.
    let vless_uuid = Uuid::new_v4();
    let tuic_uuid = Uuid::new_v4();
    let naive_username = Uuid::new_v4().simple().to_string()[..10].to_string();
    let naive_password = Uuid::new_v4().simple().to_string()[..16].to_string();
    let hy2_password = Uuid::new_v4().simple().to_string()[..16].to_string();

    let result = db::upsert_user(
        &state.db,
        payload.tg_id,
        &payload.tier,
        expire_at,
        Some(vless_uuid),
        Some(naive_username),
        Some(naive_password),
        Some(hy2_password),
        Some(tuic_uuid),
        Some(Uuid::new_v4().simple().to_string()[..16].to_string()),
        &Uuid::new_v4().simple().to_string(),
    )
    .await;

    match result {
        Ok(_) => StatusCode::OK.into_response(),
        Err(e) => {
            log::error!("Ошибка создания/обновления пользователя: {}", e);
            StatusCode::INTERNAL_SERVER_ERROR.into_response()
        }
    }
}

pub async fn get_user(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(tg_id): Path<i64>,
) -> impl IntoResponse {
    let token = extract_bearer(&headers).unwrap_or_default();
    if token != state.bot_secret {
        return StatusCode::UNAUTHORIZED.into_response();
    }

    match db::get_user(&state.db, tg_id).await {
        Ok(Some(user)) => (StatusCode::OK, Json(user)).into_response(),
        Ok(None) => StatusCode::NOT_FOUND.into_response(),
        Err(e) => {
            log::error!("Ошибка запроса пользователя: {}", e);
            StatusCode::INTERNAL_SERVER_ERROR.into_response()
        }
    }
}

pub async fn delete_user(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(tg_id): Path<i64>,
) -> impl IntoResponse {
    let token = extract_bearer(&headers).unwrap_or_default();
    if token != state.bot_secret {
        return StatusCode::UNAUTHORIZED.into_response();
    }

    match db::delete_user(&state.db, tg_id).await {
        Ok(rows) if rows > 0 => StatusCode::OK.into_response(),
        Ok(_) => StatusCode::NOT_FOUND.into_response(),
        Err(e) => {
            log::error!("Ошибка удаления пользователя: {}", e);
            StatusCode::INTERNAL_SERVER_ERROR.into_response()
        }
    }
}

// -----------------------------------------------------------------
// ПЛАТЕЖИ (API для Бота/Админа)
// -----------------------------------------------------------------

pub async fn approve_payment(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(payment_id): Path<i32>,
) -> impl IntoResponse {
    let token = extract_bearer(&headers).unwrap_or_default();
    if token != state.bot_secret {
        return StatusCode::UNAUTHORIZED.into_response();
    }

    let tg_id_opt = match db::update_payment_status(&state.db, payment_id, "approved").await {
        Ok(id) => id,
        Err(e) => {
            log::error!("Ошибка обновления статуса платежа: {}", e);
            return StatusCode::INTERNAL_SERVER_ERROR.into_response();
        }
    };

    if let Some(tg_id) = tg_id_opt {
        let expire_at = (Utc::now() + Duration::days(30)).naive_utc();
        let update_user_result = sqlx::query!(
            "UPDATE users SET tier = 'premium', expire_at = $1 WHERE tg_id = $2",
            expire_at,
            tg_id
        )
        .execute(&state.db)
        .await;

        if let Err(e) = update_user_result {
            log::error!("Ошибка начисления премиума после оплаты: {}", e);
            return StatusCode::INTERNAL_SERVER_ERROR.into_response();
        }
        StatusCode::OK.into_response()
    } else {
        StatusCode::NOT_FOUND.into_response()
    }
}

pub async fn reject_payment(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(payment_id): Path<i32>,
) -> impl IntoResponse {
    let token = extract_bearer(&headers).unwrap_or_default();
    if token != state.bot_secret {
        return StatusCode::UNAUTHORIZED.into_response();
    }

    match db::update_payment_status(&state.db, payment_id, "rejected").await {
        Ok(Some(_)) => StatusCode::OK.into_response(),
        Ok(None) => StatusCode::NOT_FOUND.into_response(),
        Err(e) => {
            log::error!("Ошибка отклонения платежа: {}", e);
            StatusCode::INTERNAL_SERVER_ERROR.into_response()
        }
    }
}

// -----------------------------------------------------------------
// УЗЛЫ (API для Бота/Админа)
// -----------------------------------------------------------------

pub async fn create_node(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(payload): Json<NodeCreateRequest>,
) -> impl IntoResponse {
    let token = extract_bearer(&headers).unwrap_or_default();
    if token != state.bot_secret {
        return StatusCode::UNAUTHORIZED.into_response();
    }

    let join_token = Uuid::new_v4().simple().to_string();

    match db::create_node(
        &state.db,
        &payload.name,
        &payload.address,
        &payload.node_type,
        &join_token,
        payload.upstream_node_id,
    )
    .await
    {
        Ok(node) => (StatusCode::CREATED, Json(node)).into_response(),
        Err(e) => {
            log::error!("Ошибка создания узла: {}", e);
            StatusCode::INTERNAL_SERVER_ERROR.into_response()
        }
    }
}

pub async fn delete_node(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(id): Path<i32>,
) -> impl IntoResponse {
    let token = extract_bearer(&headers).unwrap_or_default();
    if token != state.bot_secret {
        return StatusCode::UNAUTHORIZED.into_response();
    }

    match db::delete_node(&state.db, id).await {
        Ok(rows) if rows > 0 => StatusCode::OK.into_response(),
        Ok(_) => StatusCode::NOT_FOUND.into_response(),
        Err(e) => {
            log::error!("Ошибка удаления узла: {}", e);
            StatusCode::INTERNAL_SERVER_ERROR.into_response()
        }
    }
}

// -----------------------------------------------------------------
// КОНФИГУРАЦИЯ (Pull-модель для nup)
// -----------------------------------------------------------------

pub async fn get_nup_config(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> impl IntoResponse {
    let join_token = extract_bearer(&headers).unwrap_or_default();
    if join_token.is_empty() {
        return StatusCode::UNAUTHORIZED.into_response();
    }

    let node = match db::get_node_by_token(&state.db, &join_token).await {
        Ok(Some(n)) => n,
        Ok(None) => return StatusCode::UNAUTHORIZED.into_response(), // Невалидный токен
        Err(e) => {
            log::error!("Ошибка поиска узла по токену: {}", e);
            return StatusCode::INTERNAL_SERVER_ERROR.into_response();
        }
    };

    let users = match db::get_all_users(&state.db).await {
        Ok(u) => u,
        Err(e) => {
            log::error!("Ошибка выгрузки пользователей для nup: {}", e);
            return StatusCode::INTERNAL_SERVER_ERROR.into_response();
        }
    };

    let upstream_node = if let Some(upstream_id) = node.upstream_node_id {
        match db::get_node_by_id(&state.db, upstream_id).await {
            Ok(Some(un)) => Some(un),
            Ok(None) => {
                log::warn!(
                    "Узел {} ссылается на несуществующий upstream {}",
                    node.id,
                    upstream_id
                );
                None
            }
            Err(e) => {
                log::error!("Ошибка запроса upstream-узла: {}", e);
                None
            }
        }
    } else {
        None
    };

    let over_limit_ids = match db::get_over_limit_tg_ids(&state.db, PREMIUM_MONTHLY_LIMIT_BYTES).await {
        Ok(ids) => ids,
        Err(e) => {
            log::error!("Ошибка запроса лимитов трафика: {}", e);
            return StatusCode::INTERNAL_SERVER_ERROR.into_response();
        }
    };

    let mut users = users;
    for user in users.iter_mut() {
        user.tier = effective_tier(&user.tier, over_limit_ids.contains(&user.tg_id)).to_string();
    }

    let response = NupConfigResponse {
        node,
        users,
        upstream_node,
    };

    (StatusCode::OK, Json(response)).into_response()
}

pub async fn report_traffic(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(reports): Json<Vec<TrafficReport>>,
) -> impl IntoResponse {
    let join_token = extract_bearer(&headers).unwrap_or_default();
    match db::get_node_by_token(&state.db, &join_token).await {
        Ok(Some(_)) => {}
        Ok(None) => return StatusCode::UNAUTHORIZED.into_response(),
        Err(e) => {
            log::error!("Ошибка поиска узла по токену: {}", e);
            return StatusCode::INTERNAL_SERVER_ERROR.into_response();
        }
    }

    for report in reports {
        if report.bytes <= 0 {
            continue;
        }

        if let Err(e) = db::add_traffic(&state.db, report.tg_id, report.bytes).await {
            log::error!("Ошибка учёта трафика юзера {}: {}", report.tg_id, e);
            return StatusCode::INTERNAL_SERVER_ERROR.into_response();
        }

        let is_premium = match db::get_user(&state.db, report.tg_id).await {
            Ok(Some(user)) => user.tier == "premium",
            Ok(None) => false,
            Err(e) => {
                log::error!("Ошибка запроса пользователя: {}", e);
                return StatusCode::INTERNAL_SERVER_ERROR.into_response();
            }
        };
        if !is_premium {
            continue;
        }

        match db::mark_limit_crossed(&state.db, report.tg_id, PREMIUM_MONTHLY_LIMIT_BYTES).await {
            Ok(true) => {
                if let Err(e) =
                    db::insert_notification(&state.db, report.tg_id, LIMIT_REACHED_MESSAGE).await
                {
                    log::error!("Ошибка постановки уведомления о лимите: {}", e);
                }
            }
            Ok(false) => {}
            Err(e) => {
                log::error!("Ошибка проверки лимита трафика: {}", e);
                return StatusCode::INTERNAL_SERVER_ERROR.into_response();
            }
        }
    }

    StatusCode::OK.into_response()
}

// -----------------------------------------------------------------
// УВЕДОМЛЕНИЯ (API для Бота)
// -----------------------------------------------------------------

pub async fn get_notifications(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> impl IntoResponse {
    let token = extract_bearer(&headers).unwrap_or_default();
    if token != state.bot_secret {
        return StatusCode::UNAUTHORIZED.into_response();
    }

    match db::drain_notifications(&state.db).await {
        Ok(notifications) => (StatusCode::OK, Json(notifications)).into_response(),
        Err(e) => {
            log::error!("Ошибка выборки уведомлений: {}", e);
            StatusCode::INTERNAL_SERVER_ERROR.into_response()
        }
    }
}

// -----------------------------------------------------------------
// ПОДПИСКА (Для клиентов sing-box/xray)
// -----------------------------------------------------------------

pub async fn get_sub(
    State(state): State<AppState>,
    ConnectInfo(addr): ConnectInfo<SocketAddr>,
    Path(token): Path<String>,
) -> impl IntoResponse {
    let mut user = match db::get_user_by_sub_token(&state.db, &token).await {
        Ok(Some(u)) => u,
        Ok(None) => return StatusCode::NOT_FOUND.into_response(),
        Err(e) => {
            log::error!("Ошибка запроса пользователя по sub_token: {}", e);
            return StatusCode::INTERNAL_SERVER_ERROR.into_response();
        }
    };

    if let Some(expire_at) = user.expire_at
        && expire_at < chrono::Local::now().naive_local()
    {
        return StatusCode::FORBIDDEN.into_response();
    }

    let ip_count = match db::record_sub_access(&state.db, user.tg_id, &addr.ip().to_string()).await
    {
        Ok(count) => count,
        Err(e) => {
            log::error!("Ошибка учёта IP подписки: {}", e);
            return StatusCode::INTERNAL_SERVER_ERROR.into_response();
        }
    };

    if ip_count >= SUB_IP_LIMIT {
        if let Err(e) = rotate_and_notify(&state.db, user.tg_id).await {
            log::error!("Ошибка ротации кредов юзера {}: {}", user.tg_id, e);
            return StatusCode::INTERNAL_SERVER_ERROR.into_response();
        }
        return StatusCode::FORBIDDEN.into_response();
    }

    let over_limit = match db::get_over_limit_tg_ids(&state.db, PREMIUM_MONTHLY_LIMIT_BYTES).await {
        Ok(ids) => ids.contains(&user.tg_id),
        Err(e) => {
            log::error!("Ошибка запроса лимитов трафика: {}", e);
            return StatusCode::INTERNAL_SERVER_ERROR.into_response();
        }
    };
    user.tier = effective_tier(&user.tier, over_limit).to_string();

    let nodes = match db::get_all_nodes(&state.db).await {
        Ok(n) => n,
        Err(e) => {
            log::error!("Ошибка запроса узлов: {}", e);
            return StatusCode::INTERNAL_SERVER_ERROR.into_response();
        }
    };

    let mut configs = Vec::new();
    for node in nodes {
        configs.extend(build_sub_configs(&user, &node.address));
    }

    let combined = configs.join("\n");
    let encoded = base64::engine::general_purpose::STANDARD.encode(&combined);

    (
        StatusCode::OK,
        [("Content-Type", "text/plain; charset=utf-8")],
        encoded,
    )
        .into_response()
}

// Перевыпускает все креды и sub_token: старые конфиги на устройствах
// умирают, как только nup перечитает конфиг (до минуты).
async fn rotate_and_notify(pool: &PgPool, tg_id: i64) -> Result<(), sqlx::Error> {
    db::rotate_user_credentials(
        pool,
        tg_id,
        Uuid::new_v4(),
        &Uuid::new_v4().simple().to_string()[..10],
        &Uuid::new_v4().simple().to_string()[..16],
        &Uuid::new_v4().simple().to_string()[..16],
        Uuid::new_v4(),
        &Uuid::new_v4().simple().to_string()[..16],
        &Uuid::new_v4().simple().to_string(),
    )
    .await?;
    db::clear_sub_access(pool, tg_id).await?;
    db::insert_notification(pool, tg_id, SHARING_BLOCKED_MESSAGE).await?;

    Ok(())
}
