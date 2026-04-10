-- migrations/000017_local_government_centers.down.sql

DROP INDEX IF EXISTS idx_local_government_centers_country_state;
DROP INDEX IF EXISTS idx_local_government_centers_state_lga;

ALTER TABLE IF EXISTS local_government_centers
    DROP CONSTRAINT IF EXISTS uq_local_government_centers_country_state_lga;

DROP TABLE IF EXISTS local_government_centers;