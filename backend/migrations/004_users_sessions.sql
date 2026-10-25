CREATE TABLE users (
    username      TEXT PRIMARY KEY,
    password_hash TEXT NOT NULL,
    client_id     TEXT NOT NULL REFERENCES api_clients (client_id),
    active        BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    client_id  TEXT NOT NULL REFERENCES api_clients (client_id),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX sessions_expiry_idx ON sessions (expires_at);
