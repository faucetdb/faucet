# Filter Syntax

Faucet implements a DreamFactory-compatible filter language for querying records. Filters are passed as the `filter` query parameter and are compiled into parameterized SQL `WHERE` clauses.

All filter values are parameterized to prevent SQL injection. Column and table names are validated against an allowlist of safe identifier characters.

## Quick Reference

```
filter=status = 'active'
filter=age > 21 AND status = 'active'
filter=name LIKE 'J%' OR name LIKE 'K%'
filter=status IN ('active', 'pending', 'trial')
filter=created_at BETWEEN '2025-01-01' AND '2025-12-31'
filter=email IS NOT NULL
filter=name CONTAINS 'smith'
```

## Comparison Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `=` | Equal | `status = 'active'` |
| `!=` | Not equal | `status != 'deleted'` |
| `<>` | Not equal (alt) | `status <> 'deleted'` |
| `>` | Greater than | `age > 21` |
| `>=` | Greater than or equal | `age >= 18` |
| `<` | Less than | `price < 100` |
| `<=` | Less than or equal | `price <= 99.99` |

### Examples

```bash
# Exact match (string)
curl "http://localhost:8080/api/v1/mydb/_table/users?filter=status%20%3D%20'active'"

# Numeric comparison
curl "http://localhost:8080/api/v1/mydb/_table/products?filter=price%20%3E%2050"

# Not equal
curl "http://localhost:8080/api/v1/mydb/_table/users?filter=role%20!%3D%20'guest'"
```

## LIKE / NOT LIKE

Pattern matching with `%` (any characters) and `_` (single character) wildcards.

| Operator | Description | Example |
|----------|-------------|---------|
| `LIKE` | Pattern match | `name LIKE 'J%'` |
| `NOT LIKE` | Negated pattern match | `name NOT LIKE '%test%'` |

### Examples

```
# Names starting with "J"
filter=name LIKE 'J%'

# Email addresses at gmail
filter=email LIKE '%@gmail.com'

# 5-character names
filter=name LIKE '_____'

# Names NOT containing "test"
filter=name NOT LIKE '%test%'
```

## CONTAINS, STARTS WITH, ENDS WITH

Convenience operators that translate to `LIKE` patterns internally.

| Operator | Translates to | Example |
|----------|---------------|---------|
| `CONTAINS` | `LIKE '%value%'` | `name CONTAINS 'smith'` |
| `STARTS WITH` | `LIKE 'value%'` | `name STARTS WITH 'John'` |
| `ENDS WITH` | `LIKE '%value'` | `email ENDS WITH '@gmail.com'` |

These operators require string values.

### Examples

```
# Find records where name contains "smith" (case-sensitive)
filter=name CONTAINS 'smith'

# Find records where name starts with "John"
filter=name STARTS WITH 'John'

# Find records where email ends with a domain
filter=email ENDS WITH '@company.com'
```

## IN / NOT IN

Test membership in a list of values.

| Operator | Description | Example |
|----------|-------------|---------|
| `IN` | Value in list | `status IN ('active', 'pending')` |
| `NOT IN` | Value not in list | `status NOT IN ('deleted', 'banned')` |

### Examples

```
# Status is one of several values
filter=status IN ('active', 'pending', 'trial')

# Numeric IDs
filter=category_id IN (1, 2, 3, 4)

# Exclude specific statuses
filter=status NOT IN ('deleted', 'banned')
```

## BETWEEN / NOT BETWEEN

Test whether a value falls within an inclusive range.

| Operator | Description | Example |
|----------|-------------|---------|
| `BETWEEN` | Within range (inclusive) | `age BETWEEN 18 AND 65` |
| `NOT BETWEEN` | Outside range | `price NOT BETWEEN 10 AND 20` |

The `AND` keyword within `BETWEEN` is part of the range syntax, not a boolean operator.

### Examples

```
# Age range
filter=age BETWEEN 18 AND 65

# Date range
filter=created_at BETWEEN '2025-01-01' AND '2025-12-31'

# Price outside a range
filter=price NOT BETWEEN 10.00 AND 20.00

# Numeric range
filter=score BETWEEN 80 AND 100
```

## IS NULL / IS NOT NULL

Test for null values.

| Operator | Description | Example |
|----------|-------------|---------|
| `IS NULL` | Value is null | `deleted_at IS NULL` |
| `IS NOT NULL` | Value is not null | `email IS NOT NULL` |

### Examples

```
# Records that have not been soft-deleted
filter=deleted_at IS NULL

# Records with an email address
filter=email IS NOT NULL
```

## Boolean Operators

Combine conditions with `AND`, `OR`, and `NOT`.

| Operator | Description | Precedence |
|----------|-------------|------------|
| `NOT` | Negate condition | Highest |
| `AND` | Both conditions must be true | Medium |
| `OR` | Either condition must be true | Lowest |

### Precedence

`NOT` binds tighter than `AND`, which binds tighter than `OR`. Use parentheses to override:

```
# AND has higher precedence than OR:
# This means: (status='active' AND age>21) OR role='admin'
filter=status = 'active' AND age > 21 OR role = 'admin'

# Use parentheses to change grouping:
# This means: status='active' AND (age>21 OR role='admin')
filter=status = 'active' AND (age > 21 OR role = 'admin')
```

### Examples

```
# AND: both conditions
filter=status = 'active' AND age > 21

# OR: either condition
filter=role = 'admin' OR role = 'superadmin'

# NOT: negate a condition
filter=NOT status = 'deleted'

# Complex combination with parentheses
filter=(status = 'active' OR status = 'trial') AND age >= 18

# Multiple ANDs
filter=status = 'active' AND country = 'US' AND age > 21
```

## Parentheses

Use parentheses to control operator grouping:

```
# Without parentheses: A AND B OR C = (A AND B) OR C
filter=active = 1 AND type = 'user' OR role = 'admin'

# With parentheses: A AND (B OR C)
filter=active = 1 AND (type = 'user' OR role = 'admin')

# Nested parentheses
filter=(status = 'active' OR status = 'trial') AND (age >= 18 AND age <= 65)
```

## Value Types

### Strings

Enclose in single quotes. Use `''` (two single quotes) to escape a literal quote:

```
filter=name = 'O''Brien'
filter=status = 'active'
```

### Numbers

Integers and decimals are supported. Negative numbers are allowed:

```
filter=age = 21
filter=price = 29.99
filter=temperature > -10
filter=balance >= 0
```

### Qualified Column Names

Column names can be qualified with table or schema prefixes:

```
filter=users.status = 'active'
filter=public.users.email IS NOT NULL
```

Up to three parts are supported: `schema.table.column`.

## URL Encoding

When passing filters as query parameters, special characters must be URL-encoded:

| Character | Encoded |
|-----------|---------|
| space | `%20` or `+` |
| `=` | `%3D` |
| `'` | `%27` |
| `(` | `%28` |
| `)` | `%29` |
| `>` | `%3E` |
| `<` | `%3C` |
| `!` | `%21` |
| `%` | `%25` |
| `,` | `%2C` |

### Examples

```bash
# filter=status = 'active' AND age > 21
curl "http://localhost:8080/api/v1/mydb/_table/users?filter=status%20%3D%20%27active%27%20AND%20age%20%3E%2021"

# filter=name LIKE 'J%'
curl "http://localhost:8080/api/v1/mydb/_table/users?filter=name%20LIKE%20%27J%25%27"

# filter=id IN (1, 2, 3)
curl "http://localhost:8080/api/v1/mydb/_table/users?filter=id%20IN%20%281%2C%202%2C%203%29"
```

Tip: Many HTTP clients (like `curl` with `--data-urlencode` or JavaScript's `encodeURIComponent`) handle URL encoding automatically.

## Database-Specific Placeholders

The filter parser generates parameterized SQL using the appropriate placeholder style for each database:

| Database | Placeholder style | Example |
|----------|------------------|---------|
| PostgreSQL | `$1, $2, $3` | `age > $1 AND status = $2` |
| MySQL | `?, ?, ?` | `age > ? AND status = ?` |
| SQL Server | `@p1, @p2, @p3` | `age > @p1 AND status = @p2` |
| Snowflake | `?, ?, ?` | `age > ? AND status = ?` |
| SQLite | `?, ?, ?` | `age > ? AND status = ?` |

This is handled automatically. You write the same filter syntax regardless of the backend database.

## Error Handling

Invalid filter expressions return a 400 Bad Request with a descriptive error message:

```json
{
  "error": {
    "code": 400,
    "message": "Invalid filter: expected value after age >, got end of filter"
  }
}
```

Common errors:

| Error | Cause |
|-------|-------|
| `unterminated string literal` | Missing closing single quote |
| `unexpected token` | Operator or keyword in wrong position |
| `expected column name` | Filter starts with a value instead of a column |
| `CONTAINS requires a string value` | Used CONTAINS with a numeric value |
| `unexpected character` | Invalid character in the filter expression |

## Complete Example

```bash
# Find active users in the US over age 21, sorted by name, page 2
curl -G "http://localhost:8080/api/v1/mydb/_table/users" \
  --data-urlencode "filter=status = 'active' AND country = 'US' AND age > 21" \
  --data-urlencode "fields=id,name,email,age" \
  --data-urlencode "order=name ASC" \
  --data-urlencode "limit=25" \
  --data-urlencode "offset=25" \
  --data-urlencode "include_count=true" \
  -H "X-API-Key: faucet_your_key_here"
```
