use chrono::{NaiveDate, NaiveDateTime};
use serde::{Deserialize, Serialize};
use sqlx::FromRow;
use uuid::Uuid;

// -----------------------------------------------------------------
// БАЗОВЫЕ МОДЕЛИ (ОТРАЖЕНИЕ ТАБЛИЦ БД)
// -----------------------------------------------------------------

#[derive(Debug, Serialize, FromRow)]
pub struct User {
    pub tg_id: i64,
    pub tier: String,
    pub expire_at: Option<NaiveDateTime>,
    pub vless_uuid: Option<Uuid>,
    pub naive_username: Option<String>,
    pub naive_password: Option<String>,
    pub hy2_password: Option<String>,
    pub tuic_uuid: Option<Uuid>,
    pub tuic_password: Option<String>,
}

#[derive(Debug, Serialize, FromRow)]
pub struct PaymentCode {
    pub tg_id: i64,
    pub code_date: NaiveDate,
    pub code: String,
}

#[derive(Debug, Serialize, FromRow)]
pub struct Node {
    pub id: i32,
    pub name: String,
    pub address: String,
    pub node_type: String,
    pub status: String,
    pub join_token: String,
    pub reality_pub_key: Option<String>,
    pub reality_short_id: Option<String>,
    pub obfs_password: Option<String>,
    pub upstream_node_id: Option<i32>,
}


// -----------------------------------------------------------------
// МОДЕЛИ ВХОДЯЩИХ ЗАПРОСОВ (API REQUESTS)
// -----------------------------------------------------------------

#[derive(Debug, Deserialize)]
pub struct UserUpsertRequest {
    pub tg_id: i64,
    pub tier: String,
    pub add_days: Option<i64>,
}

#[derive(Debug, Deserialize)]
pub struct NodeCreateRequest {
    pub name: String,
    pub address: String,
    pub node_type: String,
    pub upstream_node_id: Option<i32>,
}

#[derive(Debug, Deserialize)]
pub struct NodeKeysRequest {
    pub public_key: String,
    pub short_id: String,
}

#[derive(Debug, Deserialize)]
pub struct PaymentCreateRequest {
    pub tg_id: i64,
    pub code: String,
}

// -----------------------------------------------------------------
// МОДЕЛИ ИСХОДЯЩИХ ОТВЕТОВ (API RESPONSES)
// -----------------------------------------------------------------

#[derive(Debug, Serialize)]
pub struct NupConfigResponse {
    pub node: Node,
    pub users: Vec<User>,
    pub upstream_node: Option<Node>,
}
