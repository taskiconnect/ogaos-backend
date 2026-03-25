-- migrations/000008_create_coupon_tables.down.sql

DROP TRIGGER IF EXISTS update_coupons_updated_at ON coupons;
DROP FUNCTION IF EXISTS update_updated_at_column();

ALTER TABLE subscriptions DROP CONSTRAINT IF EXISTS fk_subscriptions_coupon;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS applied_coupon_id;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS original_price;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS discount_amount;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS discount_percentage;

DROP INDEX IF EXISTS idx_subscriptions_coupon;

DROP TABLE IF EXISTS coupon_redemptions;
DROP TABLE IF EXISTS coupons;