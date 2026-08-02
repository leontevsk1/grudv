CREATE TABLE payment_codes (
    tg_id BIGINT NOT NULL REFERENCES users(tg_id),
    code_date DATE NOT NULL,
    code VARCHAR(10) NOT NULL,
    PRIMARY KEY (tg_id, code_date)
);

ALTER TABLE payment_requests ADD COLUMN code VARCHAR(10);
