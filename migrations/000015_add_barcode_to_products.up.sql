ALTER TABLE products
    ADD COLUMN IF NOT EXISTS barcode VARCHAR(100);

CREATE UNIQUE INDEX IF NOT EXISTS idx_business_barcode
    ON products (business_id, barcode)
    WHERE barcode IS NOT NULL;