-- migrations/000002_create_business_tables.up.sql

-- ─────────────────────────────────────────────────────────
-- Update businesses table with new fields
-- ─────────────────────────────────────────────────────────

ALTER TABLE businesses
  ADD COLUMN IF NOT EXISTS slug VARCHAR(255),
  ADD COLUMN IF NOT EXISTS description TEXT,
  ADD COLUMN IF NOT EXISTS logo_url VARCHAR(500),
  ADD COLUMN IF NOT EXISTS website_url VARCHAR(500),
  ADD COLUMN IF NOT EXISTS is_profile_public BOOLEAN DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS profile_views BIGINT DEFAULT 0,
  ADD COLUMN IF NOT EXISTS is_verified BOOLEAN DEFAULT FALSE;

-- Populate slug from name for existing records (sanitised lowercase)
UPDATE businesses SET slug = LOWER(REGEXP_REPLACE(name, '[^a-zA-Z0-9]+', '-', 'g')) WHERE slug IS NULL;

ALTER TABLE businesses
  ALTER COLUMN slug SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_businesses_slug ON businesses(slug);
CREATE INDEX IF NOT EXISTS idx_businesses_category ON businesses(category);
CREATE INDEX IF NOT EXISTS idx_businesses_is_profile_public ON businesses(is_profile_public);

-- ─────────────────────────────────────────────────────────
-- Subscriptions
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS subscriptions (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  plan VARCHAR(20) NOT NULL DEFAULT 'free',
  status VARCHAR(20) NOT NULL DEFAULT 'active',
  paystack_subscription_code VARCHAR(255),
  paystack_email_token VARCHAR(255),
  current_period_start TIMESTAMP,
  current_period_end TIMESTAMP,
  grace_period_ends_at TIMESTAMP,
  max_staff INT NOT NULL DEFAULT 2,
  max_stores INT NOT NULL DEFAULT 1,
  max_products INT NOT NULL DEFAULT 20,
  max_customers INT NOT NULL DEFAULT 50,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),
  UNIQUE(business_id)
);

-- Create default free subscription for every existing business
INSERT INTO subscriptions (business_id, plan, status)
SELECT id, 'free', 'active' FROM businesses
ON CONFLICT (business_id) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_subscriptions_business ON subscriptions(business_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_status ON subscriptions(status);
CREATE INDEX IF NOT EXISTS idx_subscriptions_plan ON subscriptions(plan);

-- ─────────────────────────────────────────────────────────
-- Stores
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS stores (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  name VARCHAR(255) NOT NULL,
  description TEXT,
  street VARCHAR(255),
  city_town VARCHAR(100),
  state VARCHAR(100),
  phone VARCHAR(20),
  is_default BOOLEAN DEFAULT FALSE,
  is_active BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stores_business ON stores(business_id);
CREATE INDEX IF NOT EXISTS idx_stores_is_active ON stores(is_active);

-- ─────────────────────────────────────────────────────────
-- Products
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS products (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  store_id UUID REFERENCES stores(id) ON DELETE SET NULL,
  name VARCHAR(255) NOT NULL,
  description TEXT,
  type VARCHAR(20) NOT NULL DEFAULT 'product',
  sku VARCHAR(100),
  price BIGINT NOT NULL,
  cost_price BIGINT,
  image_url VARCHAR(500),
  track_inventory BOOLEAN DEFAULT FALSE,
  stock_quantity INT DEFAULT 0,
  low_stock_threshold INT DEFAULT 5,
  is_active BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_products_business ON products(business_id);
CREATE INDEX IF NOT EXISTS idx_products_store ON products(store_id);
CREATE INDEX IF NOT EXISTS idx_products_sku ON products(sku);
CREATE INDEX IF NOT EXISTS idx_products_is_active ON products(is_active);
CREATE INDEX IF NOT EXISTS idx_products_type ON products(type);

-- ─────────────────────────────────────────────────────────
-- Customers
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS customers (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  first_name VARCHAR(100) NOT NULL,
  last_name VARCHAR(100) NOT NULL,
  email VARCHAR(255),
  phone_number VARCHAR(20),
  address TEXT,
  notes TEXT,
  total_purchases BIGINT DEFAULT 0,
  total_orders INT DEFAULT 0,
  outstanding_debt BIGINT DEFAULT 0,
  is_active BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_customers_business ON customers(business_id);
CREATE INDEX IF NOT EXISTS idx_customers_email ON customers(email);
CREATE INDEX IF NOT EXISTS idx_customers_phone ON customers(phone_number);

-- ─────────────────────────────────────────────────────────
-- Invoices (created before sales so sales can reference it)
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS invoices (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  store_id UUID REFERENCES stores(id) ON DELETE SET NULL,
  customer_id UUID REFERENCES customers(id) ON DELETE SET NULL,
  created_by UUID NOT NULL REFERENCES users(id),
  invoice_number VARCHAR(50) NOT NULL,
  issue_date TIMESTAMP NOT NULL,
  due_date TIMESTAMP NOT NULL,
  sub_total BIGINT NOT NULL,
  discount_amount BIGINT DEFAULT 0,
  vat_rate NUMERIC(5,2) DEFAULT 0,
  vat_inclusive BOOLEAN DEFAULT FALSE,
  vat_amount BIGINT DEFAULT 0,
  wht_rate NUMERIC(5,2) DEFAULT 0,
  wht_amount BIGINT DEFAULT 0,
  total_amount BIGINT NOT NULL,
  amount_paid BIGINT DEFAULT 0,
  balance_due BIGINT DEFAULT 0,
  currency VARCHAR(5) DEFAULT 'NGN',
  status VARCHAR(20) NOT NULL DEFAULT 'draft',
  notes TEXT,
  payment_terms VARCHAR(500),
  sent_at TIMESTAMP,
  paid_at TIMESTAMP,
  converted_to_sale_id UUID,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),
  UNIQUE(invoice_number)
);

CREATE INDEX IF NOT EXISTS idx_invoices_business ON invoices(business_id);
CREATE INDEX IF NOT EXISTS idx_invoices_customer ON invoices(customer_id);
CREATE INDEX IF NOT EXISTS idx_invoices_status ON invoices(status);
CREATE INDEX IF NOT EXISTS idx_invoices_due_date ON invoices(due_date);
CREATE INDEX IF NOT EXISTS idx_invoices_number ON invoices(invoice_number);

-- ─────────────────────────────────────────────────────────
-- Invoice Items
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS invoice_items (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  invoice_id UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
  product_id UUID REFERENCES products(id) ON DELETE SET NULL,
  description VARCHAR(500) NOT NULL,
  product_sku VARCHAR(100),
  unit_price BIGINT NOT NULL,
  quantity INT NOT NULL DEFAULT 1,
  discount BIGINT DEFAULT 0,
  total_price BIGINT NOT NULL,
  vat_inclusive BOOLEAN DEFAULT FALSE,
  created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_invoice_items_invoice ON invoice_items(invoice_id);
CREATE INDEX IF NOT EXISTS idx_invoice_items_product ON invoice_items(product_id);

-- ─────────────────────────────────────────────────────────
-- Sales
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS sales (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  store_id UUID REFERENCES stores(id) ON DELETE SET NULL,
  customer_id UUID REFERENCES customers(id) ON DELETE SET NULL,
  invoice_id UUID REFERENCES invoices(id) ON DELETE SET NULL,
  recorded_by UUID NOT NULL REFERENCES users(id),
  sale_number VARCHAR(50) NOT NULL,
  receipt_number VARCHAR(50),
  vat_rate NUMERIC(5,2) DEFAULT 0,
  vat_inclusive BOOLEAN DEFAULT FALSE,
  vat_amount BIGINT DEFAULT 0,
  wht_rate NUMERIC(5,2) DEFAULT 0,
  wht_amount BIGINT DEFAULT 0,
  receipt_sent_at TIMESTAMP,
  sub_total BIGINT NOT NULL,
  discount_amount BIGINT DEFAULT 0,
  total_amount BIGINT NOT NULL,
  amount_paid BIGINT DEFAULT 0,
  balance_due BIGINT DEFAULT 0,
  payment_method VARCHAR(30) NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'completed',
  notes TEXT,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),
  UNIQUE(sale_number)
);

CREATE INDEX IF NOT EXISTS idx_sales_business ON sales(business_id);
CREATE INDEX IF NOT EXISTS idx_sales_store ON sales(store_id);
CREATE INDEX IF NOT EXISTS idx_sales_customer ON sales(customer_id);
CREATE INDEX IF NOT EXISTS idx_sales_recorded_by ON sales(recorded_by);
CREATE INDEX IF NOT EXISTS idx_sales_status ON sales(status);
CREATE INDEX IF NOT EXISTS idx_sales_created_at ON sales(created_at);

-- ─────────────────────────────────────────────────────────
-- Sale Items
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS sale_items (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  sale_id UUID NOT NULL REFERENCES sales(id) ON DELETE CASCADE,
  product_id UUID REFERENCES products(id) ON DELETE SET NULL,
  product_name VARCHAR(255) NOT NULL,
  product_sku VARCHAR(100),
  unit_price BIGINT NOT NULL,
  quantity INT NOT NULL DEFAULT 1,
  discount BIGINT DEFAULT 0,
  total_price BIGINT NOT NULL,
  created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sale_items_sale ON sale_items(sale_id);
CREATE INDEX IF NOT EXISTS idx_sale_items_product ON sale_items(product_id);

-- ─────────────────────────────────────────────────────────
-- Ledger Entries
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS ledger_entries (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  type VARCHAR(10) NOT NULL,
  amount BIGINT NOT NULL,
  balance BIGINT NOT NULL,
  description VARCHAR(500) NOT NULL,
  source_type VARCHAR(50) NOT NULL,
  source_id UUID NOT NULL,
  recorded_by UUID NOT NULL REFERENCES users(id),
  created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ledger_business ON ledger_entries(business_id);
CREATE INDEX IF NOT EXISTS idx_ledger_source ON ledger_entries(source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_ledger_created_at ON ledger_entries(created_at);
CREATE INDEX IF NOT EXISTS idx_ledger_type ON ledger_entries(type);

-- ─────────────────────────────────────────────────────────
-- Debts
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS debts (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  direction VARCHAR(20) NOT NULL,
  customer_id UUID REFERENCES customers(id) ON DELETE SET NULL,
  supplier_name VARCHAR(255),
  supplier_phone VARCHAR(20),
  description TEXT NOT NULL,
  total_amount BIGINT NOT NULL,
  amount_paid BIGINT DEFAULT 0,
  amount_due BIGINT NOT NULL,
  due_date TIMESTAMP,
  status VARCHAR(20) NOT NULL DEFAULT 'outstanding',
  notes TEXT,
  recorded_by UUID NOT NULL REFERENCES users(id),
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_debts_business ON debts(business_id);
CREATE INDEX IF NOT EXISTS idx_debts_customer ON debts(customer_id);
CREATE INDEX IF NOT EXISTS idx_debts_direction ON debts(direction);
CREATE INDEX IF NOT EXISTS idx_debts_status ON debts(status);
CREATE INDEX IF NOT EXISTS idx_debts_due_date ON debts(due_date);

-- ─────────────────────────────────────────────────────────
-- Expenses
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS expenses (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  store_id UUID REFERENCES stores(id) ON DELETE SET NULL,
  category VARCHAR(50) NOT NULL,
  description TEXT NOT NULL,
  amount BIGINT NOT NULL,
  receipt_url VARCHAR(500),
  expense_date TIMESTAMP NOT NULL,
  recorded_by UUID NOT NULL REFERENCES users(id),
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_expenses_business ON expenses(business_id);
CREATE INDEX IF NOT EXISTS idx_expenses_category ON expenses(category);
CREATE INDEX IF NOT EXISTS idx_expenses_date ON expenses(expense_date);

-- ─────────────────────────────────────────────────────────
-- Payments
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS payments (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  source_type VARCHAR(20) NOT NULL,
  source_id UUID NOT NULL,
  amount BIGINT NOT NULL,
  channel VARCHAR(30) NOT NULL,
  reference VARCHAR(255),
  status VARCHAR(20) NOT NULL DEFAULT 'successful',
  note TEXT,
  recorded_by UUID NOT NULL REFERENCES users(id),
  paid_at TIMESTAMP NOT NULL,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),
  UNIQUE(reference)
);

CREATE INDEX IF NOT EXISTS idx_payments_business ON payments(business_id);
CREATE INDEX IF NOT EXISTS idx_payments_source ON payments(source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_payments_reference ON payments(reference);

-- ─────────────────────────────────────────────────────────
-- Staff Profiles
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS staff_profiles (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  position VARCHAR(100),
  department VARCHAR(100),
  salary BIGINT,
  start_date TIMESTAMP,
  emergency_contact_name VARCHAR(200),
  emergency_contact_phone VARCHAR(20),
  notes TEXT,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),
  UNIQUE(user_id)
);

CREATE INDEX IF NOT EXISTS idx_staff_profiles_business ON staff_profiles(business_id);
CREATE INDEX IF NOT EXISTS idx_staff_profiles_user ON staff_profiles(user_id);

-- ─────────────────────────────────────────────────────────
-- Job Openings
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS job_openings (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  posted_by UUID NOT NULL REFERENCES users(id),
  title VARCHAR(255) NOT NULL,
  slug VARCHAR(300) NOT NULL,
  description TEXT NOT NULL,
  requirements TEXT,
  responsibilities TEXT,
  type VARCHAR(20) NOT NULL,
  location VARCHAR(255),
  is_remote BOOLEAN DEFAULT FALSE,
  salary_range_min BIGINT,
  salary_range_max BIGINT,
  application_deadline TIMESTAMP,
  status VARCHAR(20) NOT NULL DEFAULT 'open',
  assessment_enabled BOOLEAN DEFAULT FALSE,
  assessment_category VARCHAR(50),
  pass_threshold INT DEFAULT 60,
  time_limit_minutes INT DEFAULT 30,
  application_count INT DEFAULT 0,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),
  UNIQUE(slug)
);

CREATE INDEX IF NOT EXISTS idx_job_openings_business ON job_openings(business_id);
CREATE INDEX IF NOT EXISTS idx_job_openings_status ON job_openings(status);
CREATE INDEX IF NOT EXISTS idx_job_openings_slug ON job_openings(slug);

-- ─────────────────────────────────────────────────────────
-- Recruitment Applications
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS recruitment_applications (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  job_opening_id UUID NOT NULL REFERENCES job_openings(id) ON DELETE CASCADE,
  first_name VARCHAR(100) NOT NULL,
  last_name VARCHAR(100) NOT NULL,
  email VARCHAR(255) NOT NULL,
  phone_number VARCHAR(20) NOT NULL,
  cover_letter TEXT,
  cv_url VARCHAR(500),
  status VARCHAR(30) NOT NULL DEFAULT 'new',
  review_notes TEXT,
  assessment_status VARCHAR(20) NOT NULL DEFAULT 'not_required',
  assessment_score INT,
  assessment_passed BOOLEAN,
  assessment_completed_at TIMESTAMP,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_applications_business ON recruitment_applications(business_id);
CREATE INDEX IF NOT EXISTS idx_applications_job ON recruitment_applications(job_opening_id);
CREATE INDEX IF NOT EXISTS idx_applications_email ON recruitment_applications(email);
CREATE INDEX IF NOT EXISTS idx_applications_status ON recruitment_applications(status);

-- ─────────────────────────────────────────────────────────
-- Business Identity (KYC)
-- ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS business_identities (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  cac_number VARCHAR(50),
  tin VARCHAR(50),
  bvn VARCHAR(11),
  cac_document_url VARCHAR(500),
  utility_bill_url VARCHAR(500),
  status VARCHAR(20) NOT NULL DEFAULT 'unverified',
  rejection_reason TEXT,
  verified_at TIMESTAMP,
  verified_by UUID REFERENCES platform_admins(id) ON DELETE SET NULL,
  submitted_at TIMESTAMP,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),
  UNIQUE(business_id)
);

CREATE INDEX IF NOT EXISTS idx_identity_business ON business_identities(business_id);
CREATE INDEX IF NOT EXISTS idx_identity_status ON business_identities(status);

-- Update expenses: expense_type, VAT, CapEx fields
ALTER TABLE expenses
  ADD COLUMN IF NOT EXISTS expense_type      VARCHAR(20)  NOT NULL DEFAULT 'opex',
  ADD COLUMN IF NOT EXISTS vat_inclusive     BOOLEAN      DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS vat_rate          NUMERIC(5,2) DEFAULT 0,
  ADD COLUMN IF NOT EXISTS vat_amount        BIGINT       DEFAULT 0,
  ADD COLUMN IF NOT EXISTS is_tax_deductible BOOLEAN      DEFAULT TRUE,
  ADD COLUMN IF NOT EXISTS asset_life_years  INT,
  ADD COLUMN IF NOT EXISTS asset_start_date  TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_expenses_expense_type   ON expenses(expense_type);
CREATE INDEX IF NOT EXISTS idx_expenses_tax_deductible ON expenses(is_tax_deductible);

-- Tax Summaries: monthly P&L and Nigerian tax calculation
-- CIT bands: <NGN25M=0%, NGN25M-100M=20%, >NGN100M=30%
CREATE TABLE IF NOT EXISTS tax_summaries (
  id                        UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
  business_id               UUID         NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
  period_month              INT          NOT NULL CHECK (period_month BETWEEN 1 AND 12),
  period_year               INT          NOT NULL,
  total_revenue             BIGINT       DEFAULT 0,
  digital_revenue           BIGINT       DEFAULT 0,
  total_gross_revenue       BIGINT       DEFAULT 0,
  total_cogs                BIGINT       DEFAULT 0,
  gross_profit              BIGINT       DEFAULT 0,
  total_salaries            BIGINT       DEFAULT 0,
  total_rent                BIGINT       DEFAULT 0,
  total_utilities           BIGINT       DEFAULT 0,
  total_marketing           BIGINT       DEFAULT 0,
  total_other_opex          BIGINT       DEFAULT 0,
  total_opex                BIGINT       DEFAULT 0,
  total_depreciation        BIGINT       DEFAULT 0,
  ebitda                    BIGINT       DEFAULT 0,
  net_profit_before_tax     BIGINT       DEFAULT 0,
  vat_collected             BIGINT       DEFAULT 0,
  vat_on_expenses           BIGINT       DEFAULT 0,
  vat_payable               BIGINT       DEFAULT 0,
  wht_deducted              BIGINT       DEFAULT 0,
  estimated_annual_turnover BIGINT       DEFAULT 0,
  cit_rate                  NUMERIC(5,4) DEFAULT 0,
  estimated_cit             BIGINT       DEFAULT 0,
  net_profit_after_tax      BIGINT       DEFAULT 0,
  status                    VARCHAR(10)  DEFAULT 'draft',
  generated_at              TIMESTAMP    NOT NULL,
  created_at                TIMESTAMP    DEFAULT NOW(),
  updated_at                TIMESTAMP    DEFAULT NOW(),
  UNIQUE(business_id, period_month, period_year)
);

CREATE INDEX IF NOT EXISTS idx_tax_summaries_business ON tax_summaries(business_id);
CREATE INDEX IF NOT EXISTS idx_tax_summaries_period   ON tax_summaries(period_year, period_month);