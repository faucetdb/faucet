# Faucet Benchmarks

Performance benchmarks for the Faucet API engine across supported databases.

## Prerequisites

- A running database instance (PostgreSQL, MySQL, MSSQL, or Snowflake)
- The benchmark schema loaded (see `setup.sql` or `setup_mysql.sql`)
- Faucet installed and configured with a connection to the target database

### Database Setup

**PostgreSQL:**

```bash
psql -U postgres -d bench_db -f benchmarks/setup.sql
```

**MySQL:**

```bash
mysql -u root -p bench_db < benchmarks/setup_mysql.sql
```

**MSSQL and Snowflake:** Adapt the PostgreSQL setup script to the appropriate SQL dialect for your target platform.

## Running Benchmarks

Use the built-in `faucet benchmark` command:

```bash
faucet benchmark --connection <connection_name> [options]
```

### Options

| Flag | Description | Default |
|------|-------------|---------|
| `--connection` | Named connection from your Faucet config | (required) |
| `--table` | Table to benchmark against | `bench_users` |
| `--iterations` | Number of requests per operation | `1000` |
| `--concurrency` | Concurrent requests | `10` |
| `--format` | Output format (`table`, `json`, `csv`) | `table` |

## Example Commands

### PostgreSQL

```bash
faucet benchmark --connection pg_local --table bench_users --iterations 1000 --concurrency 10
```

### MySQL

```bash
faucet benchmark --connection mysql_local --table bench_users --iterations 1000 --concurrency 10
```

### Microsoft SQL Server

```bash
faucet benchmark --connection mssql_local --table bench_users --iterations 1000 --concurrency 10
```

### Snowflake

```bash
faucet benchmark --connection snowflake_prod --table bench_users --iterations 500 --concurrency 5
```

## Interpreting Results

The benchmark output reports the following metrics for each CRUD operation (GET, POST, PUT, PATCH, DELETE):

| Metric | Description |
|--------|-------------|
| **avg_ms** | Mean response time in milliseconds |
| **p50_ms** | Median (50th percentile) response time |
| **p95_ms** | 95th percentile response time |
| **p99_ms** | 99th percentile response time |
| **rps** | Requests per second (throughput) |
| **errors** | Number of failed requests |

Lower latency and higher RPS is better. Pay attention to p95/p99 for tail latency — these indicate worst-case user experience.

## Comparison: Faucet vs DreamFactory

<!-- Update this table with actual benchmark results -->

| Operation | Faucet avg (ms) | Faucet RPS | DreamFactory avg (ms) | DreamFactory RPS | Speedup |
|-----------|-----------------|------------|-----------------------|------------------|---------|
| GET /bench_users (list) | — | — | — | — | — |
| GET /bench_users/:id | — | — | — | — | — |
| POST /bench_users | — | — | — | — | — |
| PUT /bench_users/:id | — | — | — | — | — |
| PATCH /bench_users/:id | — | — | — | — | — |
| DELETE /bench_users/:id | — | — | — | — | — |

**Test environment:** (record hardware, OS, database version, network topology here)

**Methodology:** Both engines pointed at the same database, same hardware, same concurrency settings. Each operation run for 1000 iterations with concurrency of 10.
