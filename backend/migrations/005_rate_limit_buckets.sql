CREATE TABLE rate_limit_buckets (
    client_id       TEXT PRIMARY KEY
                    REFERENCES api_clients (client_id)
                    ON DELETE CASCADE,

    tokens          NUMERIC(20, 6) NOT NULL DEFAULT 20,
    capacity        NUMERIC(20, 6) NOT NULL DEFAULT 20,
    refill_rate     NUMERIC(20, 6) NOT NULL DEFAULT 5,
    last_refill_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT rate_limit_tokens_nonnegative CHECK (tokens >= 0),
    CONSTRAINT rate_limit_capacity_positive CHECK (capacity > 0),
    CONSTRAINT rate_limit_refill_positive CHECK (refill_rate > 0),
    CONSTRAINT rate_limit_tokens_within_capacity CHECK (tokens <= capacity)
);
