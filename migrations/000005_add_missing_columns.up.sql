-- migrations/000005_add_missing_columns.up.sql
-- Safe migration: all additions use IF NOT EXISTS so they can be run
-- on any environment regardless of which columns already exist.

-- ─── businesses: address fields (missing on some deployments) ────────────────
ALTER TABLE businesses
  ADD COLUMN IF NOT EXISTS street           VARCHAR(255),
  ADD COLUMN IF NOT EXISTS city_town        VARCHAR(100),
  ADD COLUMN IF NOT EXISTS local_government VARCHAR(100),
  ADD COLUMN IF NOT EXISTS state            VARCHAR(100),
  ADD COLUMN IF NOT EXISTS country          VARCHAR(100),
  ADD COLUMN IF NOT EXISTS referral_code_used VARCHAR(50);

-- ─── businesses: storefront gallery and video (new features) ─────────────────
ALTER TABLE businesses
  ADD COLUMN IF NOT EXISTS gallery_image_urls   TEXT    NOT NULL DEFAULT '[]',
  ADD COLUMN IF NOT EXISTS storefront_video_url VARCHAR(500);

-- ─── digital_products: gallery images and expiry (new features) ──────────────
ALTER TABLE digital_products
  ADD COLUMN IF NOT EXISTS gallery_image_urls TEXT      NOT NULL DEFAULT '[]',
  ADD COLUMN IF NOT EXISTS expires_at         TIMESTAMP;

-- Backfill expires_at for existing products (180 days from created_at)
UPDATE digital_products
SET expires_at = created_at + INTERVAL '180 days'
WHERE expires_at IS NULL;

-- Index for the expiry scheduler query
CREATE INDEX IF NOT EXISTS idx_digital_products_expires_at
  ON digital_products(expires_at)
  WHERE is_published = TRUE;
