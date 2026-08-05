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
    pub config: Option<sqlx::types::Json<NodeConfig>>,
}

// -----------------------------------------------------------------
// ДЕКЛАРАТИВНАЯ КОНФИГУРАЦИЯ УЗЛА (JSONB, nodes.config)
// -----------------------------------------------------------------
// Описывает форму sing-box конфига (inbounds/outbounds/route) для узлов
// reality/web. Секреты (reality private_key, TLS-сертификаты) сюда не
// попадают никогда — их подставляет nup локально post-hoc. relay-узлы
// эту схему пока не используют (см. buildRelayConfig в nup/template.go).

#[derive(Debug, Serialize, Deserialize, Clone)]
#[serde(deny_unknown_fields)]
pub struct NodeConfig {
    pub version: i32,
    pub log_level: String,
    pub inbounds: Vec<InboundConfig>,
    pub outbounds: Vec<OutboundConfig>,
    pub route: RouteConfig,
}

#[derive(Debug, Serialize, Deserialize, Clone)]
#[serde(deny_unknown_fields)]
pub struct InboundConfig {
    pub tag: String,
    #[serde(rename = "type")]
    pub inbound_type: String,
    pub listen: String,
    pub listen_port: u16,
    #[serde(default)]
    pub protocol_settings: ProtocolSettings,
    #[serde(default)]
    pub transport: Option<TransportConfig>,
    #[serde(default)]
    pub tls: Option<TlsConfig>,
    pub user_ids: Vec<i64>,
    pub credential_field: CredentialField,
}

#[derive(Debug, Serialize, Deserialize, Clone, Default)]
#[serde(deny_unknown_fields)]
pub struct ProtocolSettings {
    #[serde(default)]
    pub flow: Option<String>,
    #[serde(default)]
    pub congestion_control: Option<String>,
}

#[derive(Debug, Serialize, Deserialize, Clone)]
#[serde(tag = "type", rename_all = "snake_case", deny_unknown_fields)]
pub enum TransportConfig {
    Httpupgrade { path: String },
}

#[derive(Debug, Serialize, Deserialize, Clone)]
#[serde(deny_unknown_fields)]
pub struct TlsConfig {
    pub mode: TlsMode,
    #[serde(default)]
    pub server_name: Option<String>,
    #[serde(default)]
    pub alpn: Option<Vec<String>>,
}

#[derive(Debug, Serialize, Deserialize, Clone, PartialEq)]
#[serde(rename_all = "snake_case")]
pub enum TlsMode {
    Cert,
    Reality,
}

#[derive(Debug, Serialize, Deserialize, Clone, PartialEq)]
#[serde(rename_all = "snake_case")]
pub enum CredentialField {
    Vless,
    Hy2,
    Tuic,
    Naive,
}

#[derive(Debug, Serialize, Deserialize, Clone)]
#[serde(deny_unknown_fields)]
pub struct OutboundConfig {
    pub tag: String,
    #[serde(rename = "type")]
    pub outbound_type: String,
    #[serde(default)]
    pub routing_mark: Option<i32>,
}

#[derive(Debug, Serialize, Deserialize, Clone)]
#[serde(deny_unknown_fields)]
pub struct RouteConfig {
    pub rules: Vec<serde_json::Value>,
    #[serde(rename = "final")]
    pub final_outbound: String,
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
