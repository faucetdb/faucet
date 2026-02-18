-- Faucet Benchmark Setup: PostgreSQL
-- Creates the bench_users table with 10,000 sample rows and indexes.

BEGIN;

-- Drop table if it already exists
DROP TABLE IF EXISTS bench_users;

-- Create table
CREATE TABLE bench_users (
    id         SERIAL PRIMARY KEY,
    name       VARCHAR(100) NOT NULL,
    email      VARCHAR(200) NOT NULL,
    age        INT NOT NULL,
    status     VARCHAR(20) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Insert 10,000 sample rows
INSERT INTO bench_users (name, email, age, status, created_at, updated_at)
SELECT
    'user_' || gs                                    AS name,
    'user_' || gs || '@bench.example.com'            AS email,
    18 + (gs % 60)                                   AS age,
    CASE (gs % 4)
        WHEN 0 THEN 'active'
        WHEN 1 THEN 'inactive'
        WHEN 2 THEN 'pending'
        WHEN 3 THEN 'suspended'
    END                                              AS status,
    NOW() - (INTERVAL '1 day' * (gs % 365))          AS created_at,
    NOW() - (INTERVAL '1 hour' * (gs % 720))         AS updated_at
FROM generate_series(1, 10000) AS gs;

-- Create indexes
CREATE INDEX idx_bench_users_email      ON bench_users (email);
CREATE INDEX idx_bench_users_status     ON bench_users (status);
CREATE INDEX idx_bench_users_created_at ON bench_users (created_at);

COMMIT;
