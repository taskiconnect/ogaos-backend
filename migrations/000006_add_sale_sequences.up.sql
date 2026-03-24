-- migrations/000006_add_sale_sequences.up.sql

CREATE TABLE IF NOT EXISTS business_sale_sequences (
    business_id          UUID PRIMARY KEY REFERENCES businesses(id) ON DELETE CASCADE,
    last_sale_number     BIGINT NOT NULL DEFAULT 0,
    last_receipt_number  BIGINT NOT NULL DEFAULT 0
);

-- Backfill existing businesses so they don't start from 0
-- This seeds the counter from the real current MAX to avoid collisions
INSERT INTO business_sale_sequences (business_id, last_sale_number, last_receipt_number)
SELECT
    b.id,
    COALESCE(
        (SELECT MAX(CAST(SUBSTRING(sale_number FROM 4) AS BIGINT))
         FROM sales WHERE business_id = b.id), 0),
    COALESCE(
        (SELECT MAX(CAST(SUBSTRING(receipt_number FROM 4) AS BIGINT))
         FROM sales WHERE business_id = b.id AND receipt_number IS NOT NULL), 0)
FROM businesses b
ON CONFLICT (business_id) DO NOTHING;