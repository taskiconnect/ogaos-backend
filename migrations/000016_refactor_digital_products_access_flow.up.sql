ALTER TABLE digital_products
  DROP COLUMN IF EXISTS expires_at,
  ADD COLUMN IF NOT EXISTS fulfillment_mode VARCHAR(30) NOT NULL DEFAULT 'file_download',
  ADD COLUMN IF NOT EXISTS access_redirect_url VARCHAR(500),
  ADD COLUMN IF NOT EXISTS requires_account BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS access_duration_hours INTEGER,
  ADD COLUMN IF NOT EXISTS delivery_note TEXT;

UPDATE digital_products
SET fulfillment_mode = CASE
  WHEN type = 'course' THEN 'course_access'
  WHEN type = 'video' THEN 'external_link'
  WHEN type = 'service' THEN 'manual_delivery'
  ELSE 'file_download'
END;

ALTER TABLE digital_orders
  ADD COLUMN IF NOT EXISTS fulfillment_mode VARCHAR(30) NOT NULL DEFAULT 'file_download',
  ADD COLUMN IF NOT EXISTS fulfillment_status VARCHAR(20) NOT NULL DEFAULT 'pending',
  ADD COLUMN IF NOT EXISTS access_url VARCHAR(500),
  ADD COLUMN IF NOT EXISTS download_count INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS max_download_count INTEGER;

UPDATE digital_orders o
SET
  fulfillment_mode = COALESCE(p.fulfillment_mode, 'file_download'),
  access_url = p.access_redirect_url,
  fulfillment_status = CASE
    WHEN o.access_granted = TRUE THEN 'ready'
    ELSE 'pending'
  END
FROM digital_products p
WHERE p.id = o.digital_product_id;