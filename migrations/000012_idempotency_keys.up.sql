ALTER TABLE sales
    ADD COLUMN IF NOT EXISTS idempotency_key UUID;

CREATE UNIQUE INDEX IF NOT EXISTS idx_sales_idempotency_key
    ON sales(idempotency_key)
    WHERE idempotency_key IS NOT NULL;

ALTER TABLE products
    ADD COLUMN IF NOT EXISTS idempotency_key UUID;

CREATE UNIQUE INDEX IF NOT EXISTS idx_products_idempotency_key
    ON products(idempotency_key)
    WHERE idempotency_key IS NOT NULL;