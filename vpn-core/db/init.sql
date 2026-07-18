CREATE TABLE users (
    tg_id BIGINT PRIMARY KEY,
    tier VARCHAR(20) NOT NULL, -- 'premium', 'free', 'admin'
    expire_at TIMESTAMP,
    vless_uuid UUID,
    naive_username VARCHAR(50),
    naive_password VARCHAR(50),
    hy2_password VARCHAR(50),
    tuic_uuid UUID,
    tuic_password VARCHAR(50),
    sub_token VARCHAR(64) UNIQUE NOT NULL
);

CREATE TABLE nodes (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL,
    address VARCHAR(100) NOT NULL,
    node_type VARCHAR(20) NOT NULL, -- 'reality', 'web', 'relay'
    status VARCHAR(20) NOT NULL,    -- 'active', 'offline'
    join_token VARCHAR(100) UNIQUE NOT NULL, -- Секрет для аутентификации nup
    reality_pub_key VARCHAR(100),
    reality_short_id VARCHAR(50),
    obfs_password VARCHAR(50),
    upstream_node_id INT REFERENCES nodes(id)
);

CREATE TABLE payment_requests (
    id SERIAL PRIMARY KEY,
    tg_id BIGINT REFERENCES users(tg_id),
    status VARCHAR(20) NOT NULL,    -- 'pending', 'approved', 'rejected'
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE sub_access (
    tg_id BIGINT NOT NULL REFERENCES users(tg_id) ON DELETE CASCADE,
    ip VARCHAR(45) NOT NULL,
    seen_at TIMESTAMP NOT NULL,
    PRIMARY KEY (tg_id, ip)
);

CREATE TABLE user_traffic (
    tg_id BIGINT NOT NULL REFERENCES users(tg_id) ON DELETE CASCADE,
    month DATE NOT NULL, -- первое число месяца
    bytes BIGINT NOT NULL DEFAULT 0,
    limit_notified BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (tg_id, month)
);

CREATE TABLE notifications (
    id SERIAL PRIMARY KEY,
    tg_id BIGINT NOT NULL,
    message TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
