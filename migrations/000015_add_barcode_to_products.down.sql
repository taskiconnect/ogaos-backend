DROP INDEX IF EXISTS idx_business_barcode;

ALTER TABLE products
    DROP COLUMN IF EXISTS barcode;