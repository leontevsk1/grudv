use crate::models::{Node, Notification, User};
use chrono::NaiveDateTime;
use sqlx::PgPool;
use uuid::Uuid;

// -----------------------------------------------------------------
// ПОЛЬЗОВАТЕЛИ
// -----------------------------------------------------------------

#[allow(clippy::too_many_arguments)]
pub async fn upsert_user(
    pool: &PgPool,
    tg_id: i64,
    tier: &str,
    expire_at: Option<NaiveDateTime>,
    vless_uuid: Option<Uuid>,
    naive_username: Option<String>,
    naive_password: Option<String>,
    hy2_password: Option<String>,
    tuic_uuid: Option<Uuid>,
    tuic_password: Option<String>,
    sub_token: &str,
) -> Result<u64, sqlx::Error> {
    let result = sqlx::query!(
        r#"
        INSERT INTO users (tg_id, tier, expire_at, vless_uuid, naive_username, naive_password, hy2_password, tuic_uuid, tuic_password, sub_token)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
        ON CONFLICT (tg_id) DO UPDATE
        SET tier = EXCLUDED.tier, expire_at = EXCLUDED.expire_at
        "#,
        tg_id, tier, expire_at, vless_uuid, naive_username, naive_password, hy2_password, tuic_uuid, tuic_password, sub_token
    )
    .execute(pool)
    .await?;

    Ok(result.rows_affected())
}

pub async fn get_user_by_sub_token(
    pool: &PgPool,
    sub_token: &str,
) -> Result<Option<User>, sqlx::Error> {
    sqlx::query_as!(User, "SELECT * FROM users WHERE sub_token = $1", sub_token)
        .fetch_optional(pool)
        .await
}

#[allow(clippy::too_many_arguments)]
pub async fn rotate_user_credentials(
    pool: &PgPool,
    tg_id: i64,
    vless_uuid: Uuid,
    naive_username: &str,
    naive_password: &str,
    hy2_password: &str,
    tuic_uuid: Uuid,
    tuic_password: &str,
    sub_token: &str,
) -> Result<u64, sqlx::Error> {
    let result = sqlx::query!(
        r#"
        UPDATE users
        SET vless_uuid = $2, naive_username = $3, naive_password = $4,
            hy2_password = $5, tuic_uuid = $6, tuic_password = $7, sub_token = $8
        WHERE tg_id = $1
        "#,
        tg_id, vless_uuid, naive_username, naive_password, hy2_password, tuic_uuid, tuic_password, sub_token
    )
    .execute(pool)
    .await?;

    Ok(result.rows_affected())
}

pub async fn get_user(pool: &PgPool, tg_id: i64) -> Result<Option<User>, sqlx::Error> {
    sqlx::query_as!(User, "SELECT * FROM users WHERE tg_id = $1", tg_id)
        .fetch_optional(pool)
        .await
}

pub async fn get_all_users(pool: &PgPool) -> Result<Vec<User>, sqlx::Error> {
    sqlx::query_as!(User, "SELECT * FROM users")
        .fetch_all(pool)
        .await
}

pub async fn delete_user(pool: &PgPool, tg_id: i64) -> Result<u64, sqlx::Error> {
    let result = sqlx::query!("DELETE FROM users WHERE tg_id = $1", tg_id)
        .execute(pool)
        .await?;

    Ok(result.rows_affected())
}

// -----------------------------------------------------------------
// ПЛАТЕЖИ
// -----------------------------------------------------------------

pub async fn update_payment_status(
    pool: &PgPool,
    payment_id: i32,
    status: &str,
) -> Result<Option<i64>, sqlx::Error> {
    let record = sqlx::query!(
        r#"
        UPDATE payment_requests 
        SET status = $1 
        WHERE id = $2 
        RETURNING tg_id
        "#,
        status,
        payment_id
    )
    .fetch_optional(pool)
    .await?;

    // Использование and_then "схлопывает" Option<Option<i64>> в Option<i64>
    Ok(record.and_then(|r| r.tg_id))
}

// -----------------------------------------------------------------
// УЗЛЫ (NODES)
// -----------------------------------------------------------------

pub async fn create_node(
    pool: &PgPool,
    name: &str,
    address: &str,
    node_type: &str,
    join_token: &str,
    upstream_node_id: Option<i32>,
) -> Result<Node, sqlx::Error> {
    sqlx::query_as!(
        Node,
        r#"
        INSERT INTO nodes (name, address, node_type, status, join_token, upstream_node_id)
        VALUES ($1, $2, $3, 'active', $4, $5)
        RETURNING *
        "#,
        name,
        address,
        node_type,
        join_token,
        upstream_node_id
    )
    .fetch_one(pool)
    .await
}

pub async fn delete_node(pool: &PgPool, id: i32) -> Result<u64, sqlx::Error> {
    let result = sqlx::query!("DELETE FROM nodes WHERE id = $1", id)
        .execute(pool)
        .await?;

    Ok(result.rows_affected())
}

pub async fn get_node_by_token(
    pool: &PgPool,
    join_token: &str,
) -> Result<Option<Node>, sqlx::Error> {
    sqlx::query_as!(
        Node,
        "SELECT * FROM nodes WHERE join_token = $1",
        join_token
    )
    .fetch_optional(pool)
    .await
}

pub async fn get_node_by_id(pool: &PgPool, id: i32) -> Result<Option<Node>, sqlx::Error> {
    sqlx::query_as!(Node, "SELECT * FROM nodes WHERE id = $1", id)
        .fetch_optional(pool)
        .await
}

pub async fn get_all_nodes(pool: &PgPool) -> Result<Vec<Node>, sqlx::Error> {
    sqlx::query_as!(Node, "SELECT * FROM nodes WHERE status = 'active'")
        .fetch_all(pool)
        .await
}

// -----------------------------------------------------------------
// ДОСТУП К ПОДПИСКЕ (ДЕТЕКТ ШАРИНГА)
// -----------------------------------------------------------------

pub async fn record_sub_access(pool: &PgPool, tg_id: i64, ip: &str) -> Result<i64, sqlx::Error> {
    sqlx::query!("DELETE FROM sub_access WHERE seen_at < now() - interval '1 hour'")
        .execute(pool)
        .await?;

    sqlx::query!(
        r#"
        INSERT INTO sub_access (tg_id, ip, seen_at)
        VALUES ($1, $2, now())
        ON CONFLICT (tg_id, ip) DO UPDATE SET seen_at = now()
        "#,
        tg_id,
        ip
    )
    .execute(pool)
    .await?;

    let record = sqlx::query!(
        r#"SELECT COUNT(*) as "count!" FROM sub_access WHERE tg_id = $1"#,
        tg_id
    )
    .fetch_one(pool)
    .await?;

    Ok(record.count)
}

pub async fn clear_sub_access(pool: &PgPool, tg_id: i64) -> Result<u64, sqlx::Error> {
    let result = sqlx::query!("DELETE FROM sub_access WHERE tg_id = $1", tg_id)
        .execute(pool)
        .await?;

    Ok(result.rows_affected())
}

// -----------------------------------------------------------------
// ТРАФИК
// -----------------------------------------------------------------

pub async fn add_traffic(pool: &PgPool, tg_id: i64, bytes: i64) -> Result<u64, sqlx::Error> {
    let result = sqlx::query!(
        r#"
        INSERT INTO user_traffic (tg_id, month, bytes)
        VALUES ($1, date_trunc('month', now())::date, $2)
        ON CONFLICT (tg_id, month) DO UPDATE SET bytes = user_traffic.bytes + EXCLUDED.bytes
        "#,
        tg_id,
        bytes
    )
    .execute(pool)
    .await?;

    Ok(result.rows_affected())
}

// Возвращает true ровно один раз — когда юзер пересёк лимит в текущем месяце.
pub async fn mark_limit_crossed(
    pool: &PgPool,
    tg_id: i64,
    limit_bytes: i64,
) -> Result<bool, sqlx::Error> {
    let record = sqlx::query!(
        r#"
        UPDATE user_traffic
        SET limit_notified = TRUE
        WHERE tg_id = $1 AND month = date_trunc('month', now())::date
          AND bytes >= $2 AND limit_notified = FALSE
        RETURNING tg_id
        "#,
        tg_id,
        limit_bytes
    )
    .fetch_optional(pool)
    .await?;

    Ok(record.is_some())
}

pub async fn get_over_limit_tg_ids(
    pool: &PgPool,
    limit_bytes: i64,
) -> Result<Vec<i64>, sqlx::Error> {
    let records = sqlx::query!(
        r#"
        SELECT tg_id FROM user_traffic
        WHERE month = date_trunc('month', now())::date AND bytes >= $1
        "#,
        limit_bytes
    )
    .fetch_all(pool)
    .await?;

    Ok(records.into_iter().map(|r| r.tg_id).collect())
}

// -----------------------------------------------------------------
// УВЕДОМЛЕНИЯ (ОЧЕРЕДЬ ДЛЯ БОТА)
// -----------------------------------------------------------------

pub async fn insert_notification(
    pool: &PgPool,
    tg_id: i64,
    message: &str,
) -> Result<u64, sqlx::Error> {
    let result = sqlx::query!(
        "INSERT INTO notifications (tg_id, message) VALUES ($1, $2)",
        tg_id,
        message
    )
    .execute(pool)
    .await?;

    Ok(result.rows_affected())
}

pub async fn drain_notifications(pool: &PgPool) -> Result<Vec<Notification>, sqlx::Error> {
    sqlx::query_as!(
        Notification,
        "DELETE FROM notifications RETURNING tg_id, message"
    )
    .fetch_all(pool)
    .await
}
