ALTER TABLE digital_orders
  DROP COLUMN IF EXISTS max_download_count,
  DROP COLUMN IF EXISTS download_count,
  DROP COLUMN IF EXISTS access_url,
  DROP COLUMN IF EXISTS fulfillment_status,
  DROP COLUMN IF EXISTS fulfillment_mode;

ALTER TABLE digital_products
  DROP COLUMN IF EXISTS delivery_note,
  DROP COLUMN IF EXISTS access_duration_hours,
  DROP COLUMN IF EXISTS requires_account,
  DROP COLUMN IF EXISTS access_redirect_url,
  DROP COLUMN IF EXISTS fulfillment_mode,
  ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_digital_products_expires_at
ON digital_products (expires_at);