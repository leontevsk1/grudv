CREATE TABLE users (
    tg_id BIGINT PRIMARY KEY,
    tier VARCHAR(20) NOT NULL, -- 'premium', 'free', 'admin'
    expire_at TIMESTAMP,
    vless_uuid UUID,
    naive_username VARCHAR(50),
    naive_password VARCHAR(50),
    hy2_password VARCHAR(50),
    tuic_uuid UUID,
    tuic_password VARCHAR(50)
);

CREATE TABLE nodes (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) UNIQUE NOT NULL,
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
