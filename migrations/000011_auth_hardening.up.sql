-- migrations/000011_auth_hardening.up.sql
-- Auth hardening: separate admin refresh tokens, composite indexes, token type safety

-- ── 1. Dedicated admin refresh tokens table ───────────────────────────────────
-- Separates admin sessions from regular user sessions completely.
-- The shared refresh_tokens table now only ever holds user (non-admin) tokens.
CREATE TABLE IF NOT EXISTS admin_refresh_tokens (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    token_hash   TEXT UNIQUE NOT NULL,
    admin_id     UUID NOT NULL REFERENCES platform_admins(id) ON DELETE CASCADE,
    expires_at   TIMESTAMP NOT NULL,
    revoked      BOOLEAN DEFAULT FALSE,
    created_at   TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_admin_refresh_tokens_hash      ON admin_refresh_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_admin_refresh_tokens_admin_id  ON admin_refresh_tokens(admin_id);
CREATE INDEX IF NOT EXISTS idx_admin_refresh_tokens_expires   ON admin_refresh_tokens(expires_at);

-- ── 2. Better composite indexes for hot query paths ───────────────────────────

-- admin_otps: the hot query is WHERE admin_id = ? AND used = false ORDER BY created_at DESC
CREATE INDEX IF NOT EXISTS idx_admin_otps_admin_active
    ON admin_otps(admin_id, used, created_at DESC);

-- admin_otp_rate_limits: always queried by admin_id alone (already has UNIQUE which implies index)
-- Add partial index for the window check (last 15 min)
CREATE INDEX IF NOT EXISTS idx_admin_rate_limits_window
    ON admin_otp_rate_limits(admin_id, last_attempt_at);

-- refresh_tokens: hot path = token_hash lookup + expiry check
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_hash_active
    ON refresh_tokens(token_hash, expires_at)
    WHERE revoked = FALSE;

-- business_users: WhoAmI and Login both query user_id + is_active
CREATE INDEX IF NOT EXISTS idx_business_users_user_active
    ON business_users(user_id, is_active, created_at DESC);

-- users: login path is always LOWER(email) = ?
-- Use a functional index so Postgres can use it without a seqscan
CREATE INDEX IF NOT EXISTS idx_users_lower_email
    ON users(LOWER(email));

-- platform_admins: same pattern
CREATE INDEX IF NOT EXISTS idx_platform_admins_lower_email
    ON platform_admins(LOWER(email));

-- ── 3. Add token_type guard to refresh_tokens (belt-and-suspenders) ──────────
-- Ensures no code path can accidentally mix admin/user tokens in the same table.
ALTER TABLE refresh_tokens
    ADD COLUMN IF NOT EXISTS token_type VARCHAR(10) NOT NULL DEFAULT 'user'
        CHECK (token_type IN ('user'));

-- ── 4. Expire old unused OTPs automatically (house-keeping) ──────────────────
-- Partial index to make the cleanup worker fast.
CREATE INDEX IF NOT EXISTS idx_admin_otps_cleanup
    ON admin_otps(expires_at)
    WHERE used = FALSE;

-- ── 5. Verification token: add hashed column for security ─────────────────────
-- The plain-text verification_token column is being replaced with a hashed version.
-- We keep the column name to avoid breaking other parts; the application will now
-- store a SHA-256 / base64 hash, and compare via HashToken().
-- No schema change needed — column type is already TEXT. This comment documents intent.

COMMENT ON COLUMN users.verification_token         IS 'SHA-256+base64 hash of the verification token (raw token is emailed, never stored)';
COMMENT ON COLUMN admin_refresh_tokens.token_hash   IS 'SHA-256+base64 hash of the raw refresh token';
COMMENT ON COLUMN admin_refresh_tokens.admin_id     IS 'References platform_admins, not users';