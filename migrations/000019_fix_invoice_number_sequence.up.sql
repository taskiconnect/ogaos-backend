-- 1) Create per-business, per-period invoice sequence table
CREATE TABLE IF NOT EXISTS business_invoice_sequences (
    business_id UUID NOT NULL,
    period VARCHAR(6) NOT NULL,
    last_invoice_number BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (business_id, period)
);

-- 2) Drop old global uniqueness on invoice_number if it exists
ALTER TABLE invoices
DROP CONSTRAINT IF EXISTS invoices_invoice_number_key;

DROP INDEX IF EXISTS invoices_invoice_number_key;

-- 3) Add composite unique index so invoice_number is unique per business
CREATE UNIQUE INDEX IF NOT EXISTS idx_invoices_business_invoice_number
ON invoices (business_id, invoice_number);