ALTER TABLE invoices
ADD COLUMN IF NOT EXISTS public_token VARCHAR(64);

UPDATE invoices
SET public_token = REPLACE(gen_random_uuid()::text, '-', '')
WHERE public_token IS NULL OR public_token = '';

ALTER TABLE invoices
ALTER COLUMN public_token SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_invoices_public_token
ON invoices(public_token);

ALTER TABLE invoices
ADD COLUMN IF NOT EXISTS revision_number INTEGER NOT NULL DEFAULT 1;

ALTER TABLE invoices
ADD COLUMN IF NOT EXISTS revised_from_invoice_id UUID NULL;

ALTER TABLE invoices
ADD COLUMN IF NOT EXISTS superseded_by_invoice_id UUID NULL;

CREATE INDEX IF NOT EXISTS idx_invoices_revised_from_invoice_id
ON invoices(revised_from_invoice_id);

CREATE INDEX IF NOT EXISTS idx_invoices_superseded_by_invoice_id
ON invoices(superseded_by_invoice_id);