use crate::models::{Node, User};
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
) -> Result<u64, sqlx::Error> {
    let result = sqlx::query!(
        r#"
        INSERT INTO users (tg_id, tier, expire_at, vless_uuid, naive_username, naive_password, hy2_password, tuic_uuid, tuic_password)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        ON CONFLICT (tg_id) DO UPDATE 
        SET tier = EXCLUDED.tier, expire_at = EXCLUDED.expire_at
        "#,
        tg_id, tier, expire_at, vless_uuid, naive_username, naive_password, hy2_password, tuic_uuid, tuic_password
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

// Переустановка воркера с тем же именем узла (например, пересоздание сервера
// после смены ОС) не должна плодить дубликат — обновляем существующую
// запись новым join_token и сбрасываем reality-ключи прежнего физического
// сервера, потому что они гарантированно невалидны для нового; nup пришлёт
// актуальные при первом запуске (см. PushRealityPublicKey в nup/reality.go).
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
        ON CONFLICT (name) DO UPDATE SET
            address = EXCLUDED.address,
            node_type = EXCLUDED.node_type,
            status = 'active',
            join_token = EXCLUDED.join_token,
            upstream_node_id = EXCLUDED.upstream_node_id,
            reality_pub_key = NULL,
            reality_short_id = NULL
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

pub async fn set_node_reality_keys(
    pool: &PgPool,
    join_token: &str,
    public_key: &str,
    short_id: &str,
) -> Result<u64, sqlx::Error> {
    let result = sqlx::query!(
        "UPDATE nodes SET reality_pub_key = $1, reality_short_id = $2 WHERE join_token = $3",
        public_key,
        short_id,
        join_token
    )
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
