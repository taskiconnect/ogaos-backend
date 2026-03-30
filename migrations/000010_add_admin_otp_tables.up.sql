-- migrations/000010_add_admin_otp_tables.up.sql

-- Enable UUID support if not already enabled (for new migrations)
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Admin OTP verification table
CREATE TABLE IF NOT EXISTS admin_otps (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    admin_id UUID NOT NULL REFERENCES platform_admins(id) ON DELETE CASCADE,
    otp_hash TEXT NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    used BOOLEAN DEFAULT FALSE,
    attempts INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Admin password reset/setup tokens
CREATE TABLE IF NOT EXISTS admin_password_resets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    admin_id UUID NOT NULL REFERENCES platform_admins(id) ON DELETE CASCADE,
    token_hash TEXT UNIQUE NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    used BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Rate limiting table for OTP attempts
CREATE TABLE IF NOT EXISTS admin_otp_rate_limits (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    admin_id UUID NOT NULL REFERENCES platform_admins(id) ON DELETE CASCADE,
    attempts INT DEFAULT 1,
    last_attempt_at TIMESTAMP DEFAULT NOW(),
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(admin_id)
);

-- Add password setup fields to platform_admins table
ALTER TABLE platform_admins 
ADD COLUMN IF NOT EXISTS password_set_at TIMESTAMP,
ADD COLUMN IF NOT EXISTS password_reset_token VARCHAR(255),
ADD COLUMN IF NOT EXISTS reset_token_expires TIMESTAMP;

-- Create indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_admin_otps_admin_id ON admin_otps(admin_id);
CREATE INDEX IF NOT EXISTS idx_admin_otps_expires_at ON admin_otps(expires_at);
CREATE INDEX IF NOT EXISTS idx_admin_otps_used ON admin_otps(used);
CREATE INDEX IF NOT EXISTS idx_admin_password_resets_token ON admin_password_resets(token_hash);
CREATE INDEX IF NOT EXISTS idx_admin_password_resets_admin_id ON admin_password_resets(admin_id);
CREATE INDEX IF NOT EXISTS idx_admin_otp_rate_limits_admin_id ON admin_otp_rate_limits(admin_id);
CREATE INDEX IF NOT EXISTS idx_platform_admins_password_reset_token ON platform_admins(password_reset_token);
CREATE INDEX IF NOT EXISTS idx_platform_admins_reset_token_expires ON platform_admins(reset_token_expires);

-- Add comments for documentation
COMMENT ON TABLE admin_otps IS 'Stores OTP codes for admin 2FA authentication';
COMMENT ON COLUMN admin_otps.otp_hash IS 'SHA-256 hashed OTP code (never store plain text)';
COMMENT ON COLUMN admin_otps.expires_at IS 'OTP expires after 5 minutes';
COMMENT ON COLUMN admin_otps.attempts IS 'Track number of verification attempts (max 5)';
COMMENT ON TABLE admin_password_resets IS 'Stores password setup tokens for new admins';
COMMENT ON TABLE admin_otp_rate_limits IS 'Rate limiting for OTP requests (5 attempts per 15 minutes)';
COMMENT ON COLUMN platform_admins.password_set_at IS 'Timestamp when admin first set their password (NULL for new admins)';