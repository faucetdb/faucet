import pgLogo from '../assets/db/pg.svg';
import mysqlLogo from '../assets/db/mysql.svg';
import mariadbLogo from '../assets/db/mariadb.svg';
import mssqlLogo from '../assets/db/mssql.svg';
import oracleLogo from '../assets/db/oracle.svg';
import sqliteLogo from '../assets/db/sqlite.svg';
import snowflakeLogo from '../assets/db/snowflake.svg';

/** Backend driver ids registered by the Faucet server. */
export type DriverId = 'postgres' | 'mysql' | 'mssql' | 'oracle' | 'sqlite' | 'snowflake';

/**
 * An engine the user can pick. MariaDB is its own choice in the UI but is
 * served by the mysql driver.
 */
export type EngineId = DriverId | 'mariadb';

export type ConnectionKind = 'network' | 'file' | 'snowflake';

export interface TlsOption {
  value: string;
  label: string;
}

export interface Engine {
  id: EngineId;
  driver: DriverId;
  label: string;
  logo: string;
  /** Wordmark logos (Oracle) are wide; render them shorter. */
  wide?: boolean;
  kind: ConnectionKind;
  defaultPort?: number;
  /** Label for the "database" field (Oracle calls it a service name). */
  databaseLabel: string;
  databasePlaceholder: string;
  userPlaceholder?: string;
  tls?: { label: string; hint: string; options: TlsOption[] };
  /** Schema exposed by default when left blank, shown as a placeholder. */
  defaultSchema?: string;
  dsnExample: string;
}

export const ENGINES: Engine[] = [
  {
    id: 'postgres',
    driver: 'postgres',
    label: 'PostgreSQL',
    logo: pgLogo,
    kind: 'network',
    defaultPort: 5432,
    databaseLabel: 'Database',
    databasePlaceholder: 'app',
    userPlaceholder: 'postgres',
    tls: {
      label: 'SSL mode',
      hint: 'Use "Require" for hosted databases such as RDS, Supabase or Neon.',
      options: [
        { value: '', label: 'Prefer (default)' },
        { value: 'disable', label: 'Disable' },
        { value: 'require', label: 'Require' },
        { value: 'verify-full', label: 'Verify full' },
      ],
    },
    defaultSchema: 'public',
    dsnExample: 'postgres://user:password@localhost:5432/app?sslmode=disable',
  },
  {
    id: 'mysql',
    driver: 'mysql',
    label: 'MySQL',
    logo: mysqlLogo,
    kind: 'network',
    defaultPort: 3306,
    databaseLabel: 'Database',
    databasePlaceholder: 'app',
    userPlaceholder: 'root',
    tls: {
      label: 'TLS',
      hint: 'Use "Required" for PlanetScale, RDS and other hosted MySQL.',
      options: [
        { value: '', label: 'Off' },
        { value: 'preferred', label: 'Preferred' },
        { value: 'true', label: 'Required' },
        { value: 'skip-verify', label: 'Required, skip verification' },
      ],
    },
    dsnExample: 'user:password@tcp(localhost:3306)/app?parseTime=true',
  },
  {
    id: 'mariadb',
    driver: 'mysql',
    label: 'MariaDB',
    logo: mariadbLogo,
    kind: 'network',
    defaultPort: 3306,
    databaseLabel: 'Database',
    databasePlaceholder: 'app',
    userPlaceholder: 'root',
    tls: {
      label: 'TLS',
      hint: 'Use "Required" for hosted MariaDB such as SkySQL or RDS.',
      options: [
        { value: '', label: 'Off' },
        { value: 'preferred', label: 'Preferred' },
        { value: 'true', label: 'Required' },
        { value: 'skip-verify', label: 'Required, skip verification' },
      ],
    },
    dsnExample: 'user:password@tcp(localhost:3306)/app?parseTime=true',
  },
  {
    id: 'mssql',
    driver: 'mssql',
    label: 'SQL Server',
    logo: mssqlLogo,
    kind: 'network',
    defaultPort: 1433,
    databaseLabel: 'Database',
    databasePlaceholder: 'master',
    userPlaceholder: 'sa',
    tls: {
      label: 'Encryption',
      hint: 'Azure SQL requires "On".',
      options: [
        { value: '', label: 'Driver default' },
        { value: 'disable', label: 'Off' },
        { value: 'true', label: 'On' },
      ],
    },
    defaultSchema: 'dbo',
    dsnExample: 'sqlserver://user:password@localhost:1433?database=app',
  },
  {
    id: 'oracle',
    driver: 'oracle',
    label: 'Oracle',
    logo: oracleLogo,
    wide: true,
    kind: 'network',
    defaultPort: 1521,
    databaseLabel: 'Service name',
    databasePlaceholder: 'FREEPDB1',
    userPlaceholder: 'app',
    tls: {
      label: 'TLS',
      hint: 'Oracle Autonomous Database (TCPS, usually port 1522) requires TLS.',
      options: [
        { value: '', label: 'Off' },
        { value: 'true', label: 'On' },
      ],
    },
    dsnExample: 'oracle://user:password@localhost:1521/FREEPDB1',
  },
  {
    id: 'sqlite',
    driver: 'sqlite',
    label: 'SQLite',
    logo: sqliteLogo,
    kind: 'file',
    databaseLabel: 'File path',
    databasePlaceholder: '/data/app.db',
    dsnExample: '/data/app.db',
  },
  {
    id: 'snowflake',
    driver: 'snowflake',
    label: 'Snowflake',
    logo: snowflakeLogo,
    kind: 'snowflake',
    databaseLabel: 'Database',
    databasePlaceholder: 'ANALYTICS',
    userPlaceholder: 'FAUCET_SVC',
    defaultSchema: 'PUBLIC',
    dsnExample: 'user:password@myorg-myaccount/ANALYTICS/PUBLIC?warehouse=COMPUTE_WH',
  },
];

export function engineById(id: string): Engine {
  return ENGINES.find((e) => e.id === id) || ENGINES[0];
}

/** Best engine for a stored driver id (mysql services show as MySQL). */
export function engineForDriver(driver: string): Engine {
  return ENGINES.find((e) => e.driver === driver) || ENGINES[0];
}

/** Connection fields as accepted by the Faucet API ("connection" object). */
export interface ConnectionParams {
  host?: string;
  port?: number;
  database?: string;
  username?: string;
  password?: string;
  ssl_mode?: string;
  path?: string;
  account?: string;
  schema?: string;
  warehouse?: string;
  role?: string;
  options?: Record<string, string>;
}

/** Human summary of where a database lives, e.g. "db.internal:5432/app". */
export function describeConnection(driver: string, c?: ConnectionParams | null): string {
  if (!c) return '';
  if (driver === 'sqlite') return c.path || '';
  if (driver === 'snowflake') {
    return [c.account, [c.database, c.schema].filter(Boolean).join('.')].filter(Boolean).join(' / ');
  }
  if (!c.host) return '';
  const port = c.port ? `:${c.port}` : '';
  return `${c.host}${port}${c.database ? '/' + c.database : ''}`;
}

/** Turn a database or file name into a URL-safe API name. */
export function slugify(input: string): string {
  const base = input.split(/[\\/]/).pop() || input;
  return base
    .replace(/\.(db|sqlite3?|s3db)$/i, '')
    .toLowerCase()
    .replace(/[^a-z0-9_-]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 48);
}

export const API_NAME_PATTERN = /^[a-zA-Z0-9][a-zA-Z0-9_-]*$/;
