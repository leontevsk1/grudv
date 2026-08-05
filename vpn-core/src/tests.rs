use crate::handlers::{build_sub_configs, extract_bearer};
use crate::models::{
    CredentialField, InboundConfig, Node, NodeConfig, OutboundConfig, ProtocolSettings,
    RouteConfig, TlsConfig, TlsMode, User,
};
use axum::http::{HeaderMap, HeaderValue};
use uuid::Uuid;

fn make_user(tier: &str) -> User {
    User {
        tg_id: 1,
        tier: tier.to_string(),
        expire_at: None,
        vless_uuid: Some(Uuid::nil()),
        naive_username: None,
        naive_password: None,
        hy2_password: Some("hy2pass".to_string()),
        tuic_uuid: Some(Uuid::nil()),
        tuic_password: Some("tuicpass".to_string()),
    }
}

fn make_tls_inbound(mode: TlsMode) -> InboundConfig {
    InboundConfig {
        tag: "in-vless".to_string(),
        inbound_type: "vless".to_string(),
        listen: "::".to_string(),
        listen_port: 443,
        protocol_settings: ProtocolSettings::default(),
        transport: None,
        tls: Some(TlsConfig {
            mode,
            server_name: Some("telemetry.mozilla.org".to_string()),
            alpn: None,
        }),
        user_ids: vec![],
        credential_field: CredentialField::Vless,
    }
}

// node_type здесь используется только как признак relay (не переведён на
// декларативный конфиг); reality/web различаются наличием tls.mode в config,
// см. build_sub_configs.
fn make_node(node_type: &str) -> Node {
    let config = if node_type == "relay" {
        None
    } else {
        let mode = if node_type == "reality" {
            TlsMode::Reality
        } else {
            TlsMode::Cert
        };
        Some(sqlx::types::Json(NodeConfig {
            version: 1,
            log_level: "info".to_string(),
            inbounds: vec![make_tls_inbound(mode)],
            outbounds: vec![OutboundConfig {
                tag: "Block".to_string(),
                outbound_type: "block".to_string(),
                routing_mark: None,
            }],
            route: RouteConfig {
                rules: vec![],
                final_outbound: "Block".to_string(),
            },
        }))
    };

    Node {
        id: 1,
        name: "node".to_string(),
        address: "node.example.com".to_string(),
        node_type: node_type.to_string(),
        status: "active".to_string(),
        join_token: "token".to_string(),
        reality_pub_key: Some("pubkey".to_string()),
        reality_short_id: Some("shortid".to_string()),
        obfs_password: None,
        upstream_node_id: None,
        config,
    }
}

#[test]
fn free_user_gets_reality_vless_hy2_and_tuic() {
    let user = make_user("free");
    let node = make_node("reality");
    let configs = build_sub_configs(&user, &node);

    assert_eq!(configs.len(), 3);
    assert!(configs[0].starts_with("vless://"));
    assert!(configs[0].contains("security=reality"));
    assert!(configs[1].starts_with("hy2://"));
    assert!(configs[2].starts_with("tuic://"));
}

#[test]
fn premium_user_gets_vless_hy2_and_tuic() {
    let user = make_user("premium");
    let node = make_node("reality");
    let configs = build_sub_configs(&user, &node);

    assert_eq!(configs.len(), 3);
    assert!(configs[0].starts_with("vless://"));
    assert!(configs[1].starts_with("hy2://"));
    assert!(configs[2].starts_with("tuic://"));
}

#[test]
fn user_without_hy2_and_tuic_gets_only_vless() {
    let mut user = make_user("premium");
    user.hy2_password = None;
    user.tuic_uuid = None;
    let node = make_node("reality");

    let configs = build_sub_configs(&user, &node);

    assert_eq!(configs.len(), 1);
}

#[test]
fn reality_node_without_keys_yet_omits_vless() {
    let user = make_user("free");
    let mut node = make_node("reality");
    node.reality_pub_key = None;
    node.reality_short_id = None;

    let configs = build_sub_configs(&user, &node);

    assert_eq!(configs.len(), 2);
    assert!(configs[0].starts_with("hy2://"));
    assert!(configs[1].starts_with("tuic://"));
}

#[test]
fn web_node_gets_httpupgrade_vless() {
    let user = make_user("free");
    let node = make_node("web");

    let configs = build_sub_configs(&user, &node);

    assert!(configs[0].starts_with("vless://"));
    assert!(configs[0].contains("type=httpupgrade"));
    assert!(configs[0].contains("path=/your-secret-health-path"));
}

#[test]
fn extract_bearer_strips_prefix() {
    let mut headers = HeaderMap::new();
    headers.insert("authorization", HeaderValue::from_static("Bearer secret123"));

    assert_eq!(extract_bearer(&headers), Some("secret123".to_string()));
}

#[test]
fn extract_bearer_rejects_missing_header() {
    let headers = HeaderMap::new();
    assert_eq!(extract_bearer(&headers), None);
}

#[test]
fn extract_bearer_rejects_wrong_scheme() {
    let mut headers = HeaderMap::new();
    headers.insert("authorization", HeaderValue::from_static("Basic secret123"));

    assert_eq!(extract_bearer(&headers), None);
}
