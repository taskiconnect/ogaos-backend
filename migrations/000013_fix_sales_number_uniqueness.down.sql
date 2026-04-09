-- migrations/000013_fix_sale_number_unique_per_business.down.sql

-- ── 1. Drop the composite per-business indexes ───────────────────────────────
DROP INDEX IF EXISTS myschema.idx_sales_business_sale_number;
DROP INDEX IF EXISTS myschema.idx_sales_business_receipt_number;

-- ── 2. Restore the original global unique indexes ────────────────────────────
CREATE UNIQUE INDEX IF NOT EXISTS sales_sale_number_key
    ON myschema.sales (sale_number);
CREATE UNIQUE INDEX IF NOT EXISTS sales_receipt_number_key
    ON myschema.sales (receipt_number);