-- ─────────────────────────────────────────────────────────
-- Coupons Table (enhanced for multi-use + admin management)
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS coupons (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    code VARCHAR(50) UNIQUE NOT NULL,
    description TEXT,
    discount_type VARCHAR(10) NOT NULL DEFAULT 'percentage' CHECK (discount_type IN ('percentage')),
    discount_value INTEGER NOT NULL CHECK (discount_value BETWEEN 1 AND 100),
    applicable_plans TEXT[] NOT NULL,
    
    -- Validity period
    starts_at TIMESTAMP WITH TIME ZONE NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    
    -- Multi-use support (new)
    max_redemptions INTEGER NOT NULL DEFAULT 1,   -- 1 = single use, 0 = unlimited
    
    -- Usage tracking (one-time use per subscription, but global limit above)
    is_used BOOLEAN DEFAULT FALSE,                -- kept for backward compatibility
    used_by_business_id UUID,
    used_at TIMESTAMP WITH TIME ZONE,
    used_on_subscription_id UUID,
    
    -- Status + soft delete (new)
    is_active BOOLEAN DEFAULT TRUE,
    deleted_at TIMESTAMP WITH TIME ZONE,
    
    -- Audit fields
    created_by UUID NOT NULL REFERENCES platform_admins(id) ON DELETE RESTRICT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Indexes for performance
    CONSTRAINT fk_coupon_used_by_business FOREIGN KEY (used_by_business_id) REFERENCES businesses(id) ON DELETE SET NULL,
    CONSTRAINT fk_coupon_used_on_subscription FOREIGN KEY (used_on_subscription_id) REFERENCES subscriptions(id) ON DELETE SET NULL,
    CONSTRAINT fk_coupon_created_by FOREIGN KEY (created_by) REFERENCES platform_admins(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_coupons_code ON coupons(code);
CREATE INDEX IF NOT EXISTS idx_coupons_is_used ON coupons(is_used) WHERE is_used = false;
CREATE INDEX IF NOT EXISTS idx_coupons_validity ON coupons(starts_at, expires_at) WHERE is_used = false AND is_active = true AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_coupons_used_by_business ON coupons(used_by_business_id) WHERE is_used = true;
CREATE INDEX IF NOT EXISTS idx_coupons_used_at ON coupons(used_at DESC) WHERE is_used = true;
CREATE INDEX IF NOT EXISTS idx_coupons_deleted_at ON coupons(deleted_at) WHERE deleted_at IS NULL;

-- ─────────────────────────────────────────────────────────
-- Coupon Redemptions Table (audit trail - already perfect)
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS coupon_redemptions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    coupon_id UUID NOT NULL,
    business_id UUID NOT NULL,
    subscription_id UUID NOT NULL,
    
    -- Purchase details
    subscription_plan VARCHAR(20) NOT NULL,
    original_amount BIGINT NOT NULL,
    discount_amount BIGINT NOT NULL,
    final_amount BIGINT NOT NULL,
    
    -- Payment context
    payment_reference VARCHAR(255),
    payment_channel VARCHAR(30),
    
    -- Metadata for fraud detection
    redeemed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    ip_address INET,
    user_agent TEXT,
    
    -- Constraints
    CONSTRAINT fk_redemption_coupon FOREIGN KEY (coupon_id) REFERENCES coupons(id) ON DELETE RESTRICT,
    CONSTRAINT fk_redemption_business FOREIGN KEY (business_id) REFERENCES businesses(id) ON DELETE CASCADE,
    CONSTRAINT fk_redemption_subscription FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE CASCADE,
    
    -- One coupon per subscription
    CONSTRAINT uq_subscription_coupon UNIQUE (subscription_id)
);

CREATE INDEX IF NOT EXISTS idx_redemptions_coupon ON coupon_redemptions(coupon_id);
CREATE INDEX IF NOT EXISTS idx_redemptions_business ON coupon_redemptions(business_id);
CREATE INDEX IF NOT EXISTS idx_redemptions_subscription ON coupon_redemptions(subscription_id);
CREATE INDEX IF NOT EXISTS idx_redemptions_redeemed_at ON coupon_redemptions(redeemed_at DESC);
CREATE INDEX IF NOT EXISTS idx_redemptions_business_plan ON coupon_redemptions(business_id, subscription_plan);
CREATE INDEX IF NOT EXISTS idx_redemptions_payment_reference ON coupon_redemptions(payment_reference) WHERE payment_reference IS NOT NULL;

-- ─────────────────────────────────────────────────────────
-- Add coupon fields to subscriptions table
-- ─────────────────────────────────────────────────────────

ALTER TABLE subscriptions
ADD COLUMN IF NOT EXISTS applied_coupon_id UUID,
ADD COLUMN IF NOT EXISTS original_price BIGINT,
ADD COLUMN IF NOT EXISTS discount_amount BIGINT DEFAULT 0,
ADD COLUMN IF NOT EXISTS discount_percentage INTEGER DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_subscriptions_coupon ON subscriptions(applied_coupon_id) WHERE applied_coupon_id IS NOT NULL;

ALTER TABLE subscriptions
ADD CONSTRAINT fk_subscriptions_coupon FOREIGN KEY (applied_coupon_id) REFERENCES coupons(id) ON DELETE SET NULL;

-- ─────────────────────────────────────────────────────────
-- Trigger for updated_at
-- ─────────────────────────────────────────────────────────

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_coupons_updated_at 
    BEFORE UPDATE ON coupons 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();