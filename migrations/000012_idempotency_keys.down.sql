DROP INDEX IF EXISTS idx_sales_idempotency_key;
ALTER TABLE sales DROP COLUMN IF EXISTS idempotency_key;

DROP INDEX IF EXISTS idx_products_idempotency_key;
ALTER TABLE products DROP COLUMN IF EXISTS idempotency_key;