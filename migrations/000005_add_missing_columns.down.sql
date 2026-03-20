-- migrations/000005_add_missing_columns.down.sql
ALTER TABLE businesses
  DROP COLUMN IF EXISTS gallery_image_urls,
  DROP COLUMN IF EXISTS storefront_video_url;

ALTER TABLE digital_products
  DROP COLUMN IF EXISTS gallery_image_urls,
  DROP COLUMN IF EXISTS expires_at;

DROP INDEX IF EXISTS idx_digital_products_expires_at;
