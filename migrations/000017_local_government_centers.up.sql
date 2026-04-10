-- migrations/000017_local_government_centers.up.sql

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS local_government_centers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    country TEXT NOT NULL,
    state TEXT NOT NULL,
    local_government TEXT NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE local_government_centers
    ADD CONSTRAINT uq_local_government_centers_country_state_lga
    UNIQUE (country, state, local_government);

CREATE INDEX IF NOT EXISTS idx_local_government_centers_state_lga
    ON local_government_centers (state, local_government);

CREATE INDEX IF NOT EXISTS idx_local_government_centers_country_state
    ON local_government_centers (country, state);