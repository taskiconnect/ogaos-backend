-- migrations/000007_add_business_keywords.up.sql

-- ─── Canonical keyword list ───────────────────────────────────────────────────
-- Each unique keyword is stored exactly once (lowercased, trimmed).
-- Businesses link to keywords via the junction table below.

CREATE TABLE IF NOT EXISTS keywords (
    id         BIGSERIAL    PRIMARY KEY,
    name       VARCHAR(80)  NOT NULL,
    CONSTRAINT uq_keywords_name UNIQUE (name)
);

CREATE INDEX IF NOT EXISTS idx_keywords_name ON keywords(name);

-- ─── Junction: business ↔ keyword (many-to-many) ──────────────────────────────
CREATE TABLE IF NOT EXISTS business_keywords (
    business_id UUID   NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
    keyword_id  BIGINT NOT NULL REFERENCES keywords(id)   ON DELETE CASCADE,
    PRIMARY KEY (business_id, keyword_id)
);

CREATE INDEX IF NOT EXISTS idx_business_keywords_business ON business_keywords(business_id);
CREATE INDEX IF NOT EXISTS idx_business_keywords_keyword  ON business_keywords(keyword_id);
