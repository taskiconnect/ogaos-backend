CREATE EXTENSION IF NOT EXISTS pgcrypto;

UPDATE digital_products
SET slug = 'prd_' || substr(replace(gen_random_uuid()::text, '-', ''), 1, 12)
WHERE slug IS NULL
   OR btrim(slug) = ''
   OR slug !~ '^prd_[a-z0-9]{12}$';

DO $$
DECLARE
    duplicate_value text;
BEGIN
    LOOP
        SELECT slug
        INTO duplicate_value
        FROM digital_products
        GROUP BY slug
        HAVING COUNT(*) > 1
        LIMIT 1;

        EXIT WHEN duplicate_value IS NULL;

        WITH ranked AS (
            SELECT id,
                   ROW_NUMBER() OVER (PARTITION BY slug ORDER BY created_at ASC, id ASC) AS rn
            FROM digital_products
            WHERE slug = duplicate_value
        )
        UPDATE digital_products dp
        SET slug = 'prd_' || substr(replace(gen_random_uuid()::text, '-', ''), 1, 12)
        FROM ranked r
        WHERE dp.id = r.id
          AND r.rn > 1;
    END LOOP;
END $$;

ALTER TABLE digital_products
ALTER COLUMN slug SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_indexes
        WHERE schemaname = 'public'
          AND indexname = 'idx_digital_products_slug_unique'
    ) THEN
        CREATE UNIQUE INDEX idx_digital_products_slug_unique
            ON digital_products (slug);
    END IF;
END $$;