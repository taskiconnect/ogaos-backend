-- migrations/000004_add_staff_name_to_sales.up.sql
ALTER TABLE sales ADD COLUMN IF NOT EXISTS staff_name VARCHAR(255);
