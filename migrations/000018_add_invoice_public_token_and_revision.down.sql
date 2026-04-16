DROP INDEX IF EXISTS idx_invoices_superseded_by_invoice_id;
DROP INDEX IF EXISTS idx_invoices_revised_from_invoice_id;
DROP INDEX IF EXISTS idx_invoices_public_token;

ALTER TABLE invoices DROP COLUMN IF EXISTS superseded_by_invoice_id;
ALTER TABLE invoices DROP COLUMN IF EXISTS revised_from_invoice_id;
ALTER TABLE invoices DROP COLUMN IF EXISTS revision_number;
ALTER TABLE invoices DROP COLUMN IF EXISTS public_token;