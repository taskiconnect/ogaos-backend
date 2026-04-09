-- migrations/000013_fix_sale_number_unique_per_business.up.sql
-- Problem: sale_number and receipt_number were globally unique across ALL businesses.
-- This caused duplicate key errors when two businesses happened to generate the same
-- sale number (e.g. both at SL-000001). The fix scopes uniqueness per business.

-- ── 1. Drop the old global unique indexes ────────────────────────────────────
-- NOTE: GORM's uniqueIndex tag creates these as indexes, not constraints,
-- so DROP CONSTRAINT silently does nothing — DROP INDEX is required.
-- Schema-qualified so this works regardless of search_path configuration.
DROP INDEX IF EXISTS myschema.sales_sale_number_key;
DROP INDEX IF EXISTS myschema.sales_receipt_number_key;

-- ── 2. Create composite unique indexes (per business) ────────────────────────

-- sale_number must be unique within a business, not globally
CREATE UNIQUE INDEX IF NOT EXISTS idx_sales_business_sale_number
    ON myschema.sales (business_id, sale_number);

-- receipt_number must be unique within a business, not globally
-- Partial index: only enforced when receipt_number is not NULL
CREATE UNIQUE INDEX IF NOT EXISTS idx_sales_business_receipt_number
    ON myschema.sales (business_id, receipt_number)
    WHERE receipt_number IS NOT NULL;