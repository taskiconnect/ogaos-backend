-- 1) Remove composite per-business unique index
DROP INDEX IF EXISTS idx_invoices_business_invoice_number;

-- 2) Restore global uniqueness on invoice_number
ALTER TABLE invoices
ADD CONSTRAINT invoices_invoice_number_key UNIQUE (invoice_number);

-- 3) Drop invoice sequence table
DROP TABLE IF EXISTS business_invoice_sequences;