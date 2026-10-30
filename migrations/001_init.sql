CREATE TABLE IF NOT EXISTS rate_updates (
    id UUID PRIMARY KEY,
    base CHAR(3) NOT NULL,
    quote CHAR(3) NOT NULL,
    status TEXT NOT NULL,
    price NUMERIC(18,6),
    error TEXT,
    requested_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,

    CONSTRAINT rate_updates_status_check
        CHECK (status IN ('pending', 'in_progress', 'completed', 'failed')),
    CONSTRAINT rate_updates_pair_differs 
        CHECK (base <> quote)
);

CREATE TABLE IF NOT EXISTS rates (
    base CHAR(3) NOT NULL, 
    quote CHAR(3) NOT NULL, 
    price NUMERIC(18,6) NOT NULL, 
    updated_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT rates_pair_pk PRIMARY KEY (base, quote)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_rate_updates_in_flight 
    ON rate_updates (base, quote)
    WHERE status IN ('pending', 'in_progress');

CREATE INDEX IF NOT EXISTS idx_rate_updates_status
    ON rate_updates (status);