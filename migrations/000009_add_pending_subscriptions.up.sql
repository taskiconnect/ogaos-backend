-- ========================================================
-- Migration 000009: Pending Subscriptions for Paystack Integration
-- Purpose: Support secure, idempotent one-time subscription payments
-- ========================================================

-- ─────────────────────────────────────────────────────────
-- Pending Subscriptions Table
-- Used for:
--   - Idempotency (same reference = processed once)
--   - Race condition protection
--   - Tracking payment intent before Paystack confirmation
--   - Supporting 100% discount coupons (no Paystack call needed)
-- ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS pending_subscriptions (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    business_id       UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
    reference         VARCHAR(100) UNIQUE NOT NULL,
    plan              VARCHAR(20) NOT NULL CHECK (plan IN ('growth', 'pro')),
    period_months     INTEGER NOT NULL CHECK (period_months BETWEEN 1 AND 12),
    original_amount   BIGINT NOT NULL,                    -- in kobo (before discount)
    final_amount      BIGINT NOT NULL,                    -- in kobo (after coupon)
    coupon_code       VARCHAR(50),
    status            VARCHAR(20) NOT NULL DEFAULT 'pending', 
    -- status values: pending, completed, expired, failed
    expires_at        TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at        TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at        TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for performance and safety
CREATE INDEX IF NOT EXISTS idx_pending_subscriptions_reference 
    ON pending_subscriptions(reference);

CREATE INDEX IF NOT EXISTS idx_pending_subscriptions_business 
    ON pending_subscriptions(business_id);

CREATE INDEX IF NOT EXISTS idx_pending_subscriptions_status 
    ON pending_subscriptions(status);

CREATE INDEX IF NOT EXISTS idx_pending_subscriptions_expires_at 
    ON pending_subscriptions(expires_at);

-- Unique constraint already enforced by reference column

-- ─────────────────────────────────────────────────────────
-- Add helpful updated_at trigger
-- ─────────────────────────────────────────────────────────
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_pending_subscriptions_updated_at 
    BEFORE UPDATE ON pending_subscriptions 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

-- ─────────────────────────────────────────────────────────
-- Optional: Clean up any expired pending records older than 24h (safety)
-- This is safe to run on first migration
-- ─────────────────────────────────────────────────────────
DELETE FROM pending_subscriptions 
WHERE status = 'pending' 
  AND expires_at < NOW() - INTERVAL '24 hours';

-- ========================================================
-- End of Migration 000009
-- ========================================================