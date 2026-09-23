CREATE TABLE IF NOT EXISTS orders (
    order_id TEXT PRIMARY KEY,
    "timestamp" TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL,
    customer JSONB,
    financials JSONB,
    line_items JSONB,
    polymorphic_metadata JSONB,
    event_timeline JSONB
);

CREATE INDEX IF NOT EXISTS idx_orders_status ON orders (status);
