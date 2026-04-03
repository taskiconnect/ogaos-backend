-- migrations/000013_fix_sale_number_unique_per_business.down.sql

-- ── 1. Drop the composite per-business indexes ───────────────────────────────
DROP INDEX IF EXISTS idx_sales_business_sale_number;
DROP INDEX IF EXISTS idx_sales_business_receipt_number;

-- ── 2. Restore the original global unique constraints ────────────────────────
ALTER TABLE sales ADD CONSTRAINT sales_sale_number_key UNIQUE (sale_number);
ALTER TABLE sales ADD CONSTRAINT sales_receipt_number_key UNIQUE (receipt_number);