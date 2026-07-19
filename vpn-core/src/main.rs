mod db;
mod handlers;
mod models;
#[cfg(test)]
mod tests;

use axum::{
    Router,
    routing::{delete, get, post},
};
use sqlx::postgres::PgPoolOptions;

use handlers::AppState;

#[tokio::main]
async fn main() {
    pretty_env_logger::init();
    dotenvy::dotenv().ok();

    log::info!("Запуск vpn-core...");

    let db_url = std::env::var("DATABASE_URL").expect("DATABASE_URL не задан");
    // Секрет для авторизации запросов от бота
    let bot_secret = std::env::var("BOT_SECRET").expect("BOT_SECRET не задан");

    let pool = PgPoolOptions::new()
        .max_connections(5)
        .connect(&db_url)
        .await
        .expect("Ошибка подключения к PostgreSQL");

    log::info!("Подключение к БД установлено");

    let state = AppState {
        db: pool,
        bot_secret,
    };

    let app = Router::new()
        // Управление пользователями (для бота)
        .route("/api/v1/users", post(handlers::upsert_user))
        .route(
            "/api/v1/users/{tg_id}",
            get(handlers::get_user).delete(handlers::delete_user),
        )
        // Управление заявками на оплату (для бота/админа)
        .route(
            "/api/v1/payments/{id}/approve",
            post(handlers::approve_payment),
        )
        .route(
            "/api/v1/payments/{id}/reject",
            post(handlers::reject_payment),
        )
        // Управление узлами (для бота/админа)
        .route("/api/v1/nodes", post(handlers::create_node))
        .route("/api/v1/nodes/{id}", delete(handlers::delete_node))
        // Pull-конфигурация (для nup)
        .route("/api/v1/nup/config", get(handlers::get_nup_config))
        .route("/api/v1/nup/node-keys", post(handlers::set_node_keys))
        // Подписка для клиентов
        .route("/api/sub/{tg_id}", get(handlers::get_sub))
        .with_state(state);

    let port = 8443;
    let listener = tokio::net::TcpListener::bind(format!("0.0.0.0:{}", port))
        .await
        .unwrap();

    log::info!("vpn-core слушает порт {}", port);
    axum::serve(listener, app).await.unwrap();
}
