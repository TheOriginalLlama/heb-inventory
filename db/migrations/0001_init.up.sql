CREATE TABLE IF NOT EXISTS items (
    sku              TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    department       TEXT NOT NULL DEFAULT '',
    unit_price_cents BIGINT NOT NULL DEFAULT 0 CHECK (unit_price_cents >= 0),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS items_department_idx ON items (department);

CREATE TABLE IF NOT EXISTS stock (
    sku        TEXT NOT NULL REFERENCES items(sku) ON DELETE CASCADE,
    store_id   TEXT NOT NULL,
    quantity   BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (sku, store_id)
);

CREATE INDEX IF NOT EXISTS stock_store_idx ON stock (store_id);

CREATE TABLE IF NOT EXISTS stock_movements (
    id         BIGSERIAL PRIMARY KEY,
    sku        TEXT NOT NULL REFERENCES items(sku) ON DELETE CASCADE,
    store_id   TEXT NOT NULL,
    delta      BIGINT NOT NULL,
    reason     TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS stock_movements_sku_store_idx ON stock_movements (sku, store_id);
CREATE INDEX IF NOT EXISTS stock_movements_created_at_idx ON stock_movements (created_at DESC);
