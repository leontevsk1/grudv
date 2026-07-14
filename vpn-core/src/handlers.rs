use axum::{
    Json,
    extract::{Path, State},
    http::{HeaderMap, StatusCode},
    response::IntoResponse,
};
use base64::Engine;
use chrono::{Duration, Utc};
use sqlx::PgPool;
use uuid::Uuid;

use crate::{
    db,
    models::{NodeCreateRequest, NupConfigResponse, UserUpsertRequest},
};

#[derive(Clone)]
pub struct AppState {
    pub db: PgPool,
    pub bot_secret: String,
}

// Вспомогательная функция для извлечения Bearer-токена
fn extract_bearer(headers: &HeaderMap) -> Option<String> {
    headers
        .get("authorization")
        .and_then(|v| v.to_str().ok())
        .and_then(|s| s.strip_prefix("Bearer ").map(String::from))
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

    let response = NupConfigResponse {
        node,
        users,
        upstream_node,
    };

    (StatusCode::OK, Json(response)).into_response()
}

// -----------------------------------------------------------------
// ПОДПИСКА (Для клиентов sing-box/xray)
// -----------------------------------------------------------------

pub async fn get_sub(State(state): State<AppState>, Path(tg_id): Path<i64>) -> impl IntoResponse {
    match db::get_all_nodes(&state.db).await {
        Ok(nodes) => match db::get_user(&state.db, tg_id).await {
            Ok(Some(user)) => {
                if let Some(expire_at) = user.expire_at
                    && expire_at < chrono::Local::now().naive_local()
                {
                    return StatusCode::FORBIDDEN.into_response();
                }

                let mut configs = Vec::new();
                for node in nodes {
                    if user.tier == "free" {
                        let config = format!(
                            "vless://{}@{}?encryption=none&security=tls&type=httpupgrade&mark=100",
                            user.vless_uuid.unwrap_or_default(),
                            node.address
                        );
                        configs.push(config);
                    } else {
                        let config = format!(
                            "vless://{}@{}?encryption=none&security=tls&type=httpupgrade",
                            user.vless_uuid.unwrap_or_default(),
                            node.address
                        );
                        configs.push(config);

                        if let Some(hy2_pass) = &user.hy2_password {
                            let hy2_config = format!("hy2://{}@{}", hy2_pass, node.address);
                            configs.push(hy2_config);
                        }

                        if let Some(tuic_uuid) = user.tuic_uuid {
                            let tuic_config = format!("tuic://{}@{}", tuic_uuid, node.address);
                            configs.push(tuic_config);
                        }
                    }
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
            Ok(None) => StatusCode::NOT_FOUND.into_response(),
            Err(e) => {
                log::error!("Ошибка запроса пользователя: {}", e);
                StatusCode::INTERNAL_SERVER_ERROR.into_response()
            }
        },
        Err(e) => {
            log::error!("Ошибка запроса узлов: {}", e);
            StatusCode::INTERNAL_SERVER_ERROR.into_response()
        }
    }
}
