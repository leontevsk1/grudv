use crate::handlers::{build_sub_configs, effective_tier, extract_bearer};
use crate::models::User;
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
        sub_token: "token123".to_string(),
    }
}

#[test]
fn free_user_gets_only_vless() {
    let user = make_user("free");
    let configs = build_sub_configs(&user, "node.example.com");

    assert_eq!(configs.len(), 1);
    assert!(configs[0].starts_with("vless://"));
    assert!(!configs[0].contains("mark=100"));
}

#[test]
fn premium_user_gets_vless_hy2_and_tuic() {
    let user = make_user("premium");
    let configs = build_sub_configs(&user, "node.example.com");

    assert_eq!(configs.len(), 3);
    assert!(configs[0].starts_with("vless://"));
    assert!(configs[1].starts_with("hy2://"));
    assert!(configs[2].starts_with("tuic://"));
}

#[test]
fn premium_user_without_hy2_and_tuic_gets_only_vless() {
    let mut user = make_user("premium");
    user.hy2_password = None;
    user.tuic_uuid = None;

    let configs = build_sub_configs(&user, "node.example.com");

    assert_eq!(configs.len(), 1);
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

#[test]
fn premium_over_limit_becomes_free() {
    assert_eq!(effective_tier("premium", true), "free");
}

#[test]
fn premium_under_limit_stays_premium() {
    assert_eq!(effective_tier("premium", false), "premium");
}

#[test]
fn free_and_admin_tiers_unchanged_by_limit() {
    assert_eq!(effective_tier("free", true), "free");
    assert_eq!(effective_tier("admin", true), "admin");
}

#[test]
fn over_limit_premium_gets_only_vless() {
    let mut user = make_user("premium");
    user.tier = effective_tier(&user.tier, true).to_string();

    let configs = build_sub_configs(&user, "node.example.com");

    assert_eq!(configs.len(), 1);
    assert!(configs[0].starts_with("vless://"));
}
