CREATE TABLE payout_account_verifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    business_id UUID NOT NULL,

    bank_name VARCHAR(100) NOT NULL,
    bank_code VARCHAR(10) NOT NULL,
    account_number VARCHAR(20) NOT NULL,
    account_name VARCHAR(255) NOT NULL,

    otp_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    resend_after TIMESTAMP NOT NULL,

    attempts INT DEFAULT 0,
    max_attempts INT DEFAULT 5,

    is_verified BOOLEAN DEFAULT FALSE,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_payout_verifications_business_id 
ON payout_account_verifications(business_id);

CREATE INDEX idx_payout_verifications_expires_at 
ON payout_account_verifications(expires_at);