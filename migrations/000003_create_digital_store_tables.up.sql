-- migrations/000003_create_digital_store_tables.up.sql

-- ─────────────────────────────────────────────────────────
-- Assessment Questions (platform admin managed)
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS assessment_questions (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  category VARCHAR(50) NOT NULL,
  question_text TEXT NOT NULL,
  option_a VARCHAR(500) NOT NULL,
  option_b VARCHAR(500) NOT NULL,
  option_c VARCHAR(500) NOT NULL,
  option_d VARCHAR(500) NOT NULL,
  correct_option VARCHAR(1) NOT NULL,
  explanation TEXT,
  difficulty VARCHAR(10) NOT NULL DEFAULT 'medium',
  is_active BOOLEAN DEFAULT TRUE,
  created_by UUID NOT NULL REFERENCES platform_admins(id),
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_questions_category ON assessment_questions(category);
CREATE INDEX IF NOT EXISTS idx_questions_is_active ON assessment_questions(is_active);
CREATE INDEX IF NOT EXISTS idx_questions_difficulty ON assessment_questions(difficulty);

-- ─────────────────────────────────────────────────────────
-- Assessment Sessions
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS assessment_sessions (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  application_id UUID NOT NULL REFERENCES recruitment_applications(id) ON DELETE CASCADE,
  business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  token VARCHAR(255) NOT NULL,
  questions_snapshot JSONB NOT NULL,
  answers_snapshot JSONB,
  score INT,
  total_questions INT NOT NULL,
  correct_answers INT,
  time_limit_minutes INT NOT NULL DEFAULT 30,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  expires_at TIMESTAMP NOT NULL,
  started_at TIMESTAMP,
  completed_at TIMESTAMP,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),
  UNIQUE(application_id),
  UNIQUE(token)
);

CREATE INDEX IF NOT EXISTS idx_sessions_application ON assessment_sessions(application_id);
CREATE INDEX IF NOT EXISTS idx_sessions_business ON assessment_sessions(business_id);
CREATE INDEX IF NOT EXISTS idx_sessions_token ON assessment_sessions(token);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON assessment_sessions(status);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON assessment_sessions(expires_at);

-- ─────────────────────────────────────────────────────────
-- Digital Products
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS digital_products (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  title VARCHAR(255) NOT NULL,
  slug VARCHAR(300) NOT NULL,
  description TEXT NOT NULL,
  type VARCHAR(30) NOT NULL,
  price BIGINT NOT NULL,
  currency VARCHAR(5) DEFAULT 'NGN',
  cover_image_url VARCHAR(500),
  promo_video_url VARCHAR(500),
  file_url VARCHAR(500),
  file_size BIGINT,
  file_mime_type VARCHAR(100),
  is_published BOOLEAN DEFAULT FALSE,
  sales_count INT DEFAULT 0,
  total_revenue BIGINT DEFAULT 0,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),
  UNIQUE(business_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_digital_products_business ON digital_products(business_id);
CREATE INDEX IF NOT EXISTS idx_digital_products_slug ON digital_products(slug);
CREATE INDEX IF NOT EXISTS idx_digital_products_published ON digital_products(is_published);
CREATE INDEX IF NOT EXISTS idx_digital_products_type ON digital_products(type);

-- ─────────────────────────────────────────────────────────
-- Digital Orders
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS digital_orders (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  digital_product_id UUID NOT NULL REFERENCES digital_products(id) ON DELETE RESTRICT,
  buyer_name VARCHAR(200) NOT NULL,
  buyer_email VARCHAR(255) NOT NULL,
  buyer_phone VARCHAR(20),
  amount_paid BIGINT NOT NULL,
  platform_fee BIGINT NOT NULL,
  owner_payout_amount BIGINT NOT NULL,
  currency VARCHAR(5) DEFAULT 'NGN',
  payment_channel VARCHAR(30) NOT NULL,
  payment_reference VARCHAR(255),
  payment_status VARCHAR(20) NOT NULL DEFAULT 'pending',
  paid_at TIMESTAMP,
  access_granted BOOLEAN DEFAULT FALSE,
  access_token VARCHAR(255),
  access_expires_at TIMESTAMP,
  payout_status VARCHAR(20) NOT NULL DEFAULT 'pending',
  payout_reference VARCHAR(255),
  payout_attempts INT DEFAULT 0,
  payout_completed_at TIMESTAMP,
  payout_fail_reason TEXT,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),
  UNIQUE(payment_reference),
  UNIQUE(access_token)
);

CREATE INDEX IF NOT EXISTS idx_digital_orders_business ON digital_orders(business_id);
CREATE INDEX IF NOT EXISTS idx_digital_orders_product ON digital_orders(digital_product_id);
CREATE INDEX IF NOT EXISTS idx_digital_orders_buyer_email ON digital_orders(buyer_email);
CREATE INDEX IF NOT EXISTS idx_digital_orders_payment_status ON digital_orders(payment_status);
CREATE INDEX IF NOT EXISTS idx_digital_orders_payout_status ON digital_orders(payout_status);
CREATE INDEX IF NOT EXISTS idx_digital_orders_access_token ON digital_orders(access_token);

-- ─────────────────────────────────────────────────────────
-- Business Payout Accounts
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS business_payout_accounts (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  bank_name VARCHAR(100) NOT NULL,
  bank_code VARCHAR(10) NOT NULL,
  account_number VARCHAR(20) NOT NULL,
  account_name VARCHAR(255) NOT NULL,
  paystack_recipient_code VARCHAR(100),
  is_verified BOOLEAN DEFAULT FALSE,
  is_default BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payout_accounts_business ON business_payout_accounts(business_id);

-- ─────────────────────────────────────────────────────────
-- Custom Domains (Pro plan)
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS custom_domains (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  domain VARCHAR(255) NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  verification_token VARCHAR(100) NOT NULL,
  verified_at TIMESTAMP,
  ssl_provisioned BOOLEAN DEFAULT FALSE,
  ssl_provisioned_at TIMESTAMP,
  last_checked_at TIMESTAMP,
  fail_reason TEXT,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),
  UNIQUE(business_id),
  UNIQUE(domain)
);

CREATE INDEX IF NOT EXISTS idx_custom_domains_business ON custom_domains(business_id);
CREATE INDEX IF NOT EXISTS idx_custom_domains_domain ON custom_domains(domain);
CREATE INDEX IF NOT EXISTS idx_custom_domains_status ON custom_domains(status);