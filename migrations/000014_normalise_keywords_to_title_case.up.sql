-- Step 1: remove case-insensitive duplicates, keeping the lowest id
DELETE FROM keywords
WHERE id IN (
  SELECT k1.id
  FROM keywords k1
  JOIN keywords k2
    ON LOWER(k1.name) = LOWER(k2.name)
   AND k1.id > k2.id
);

-- Step 2: normalize remaining keywords to Title Case
UPDATE keywords
SET name = initcap(
  trim(
    regexp_replace(name, '\s+', ' ', 'g')
  )
);