-- Faucet Benchmark Setup: MySQL
-- Creates the bench_users table with 10,000 sample rows and indexes.

DROP TABLE IF EXISTS bench_users;

-- Create table
CREATE TABLE bench_users (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    name       VARCHAR(100) NOT NULL,
    email      VARCHAR(200) NOT NULL,
    age        INT NOT NULL,
    status     VARCHAR(20) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Insert 10,000 sample rows using a recursive CTE (MySQL 8.0+)
INSERT INTO bench_users (name, email, age, status, created_at, updated_at)
WITH RECURSIVE seq AS (
    SELECT 1 AS n
    UNION ALL
    SELECT n + 1 FROM seq WHERE n < 10000
)
SELECT
    CONCAT('user_', n)                              AS name,
    CONCAT('user_', n, '@bench.example.com')        AS email,
    18 + (n MOD 60)                                 AS age,
    ELT((n MOD 4) + 1,
        'active', 'inactive', 'pending', 'suspended')  AS status,
    DATE_SUB(NOW(), INTERVAL (n MOD 365) DAY)       AS created_at,
    DATE_SUB(NOW(), INTERVAL (n MOD 720) HOUR)      AS updated_at
FROM seq;

-- Create indexes
CREATE INDEX idx_bench_users_email      ON bench_users (email);
CREATE INDEX idx_bench_users_status     ON bench_users (status);
CREATE INDEX idx_bench_users_created_at ON bench_users (created_at);
