CREATE TABLE IF NOT EXISTS whatsapp_device (
    id BIGSERIAL PRIMARY KEY,
    number VARCHAR(20) NOT NULL UNIQUE,
    container_id TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    version TEXT,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_who TEXT
    );

CREATE TABLE IF NOT EXISTS whatsapp_device_webhook (
    id BIGSERIAL PRIMARY KEY,
    device_id BIGINT NOT NULL REFERENCES whatsapp_device(id),
    number VARCHAR(20) NOT NULL,
    phrase VARCHAR(100) NOT NULL,
    url TEXT NOT NULL,
    url_method TEXT NOT NULL,
    body TEXT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
    );

CREATE TABLE IF NOT EXISTS whatsapp_event_log (
    id BIGSERIAL PRIMARY KEY,
    number VARCHAR(20) NOT NULL,
    ip VARCHAR(100),
    method VARCHAR(20),
    endpoint TEXT,
    user_agent TEXT,
    status_code TEXT,
    request_body JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);


ALTER TABLE whatsapp_device_webhook
    ADD CONSTRAINT whatsapp_device_webhook_unique_rule
    UNIQUE (device_id, number, phrase, url, url_method, body);
