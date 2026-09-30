import { useState } from 'preact/hooks';

// ConnectionInput lets the user describe a database connection either as a
// single connection string (DSN) or as individual fields. The parent owns the
// value; connectionPayload() turns it into the API request fields.

export type ConnectionMode = 'fields' | 'dsn';

export interface ConnectionValue {
  mode: ConnectionMode;
  dsn: string;
  host: string;
  port: string;
  user: string;
  password: string;
  database: string;
  params: string; // key=value pairs, separated by & or new lines
}

export const emptyConnection: ConnectionValue = {
  mode: 'fields',
  dsn: '',
  host: '',
  port: '',
  user: '',
  password: '',
  database: '',
  params: '',
};

const DEFAULT_PORTS: Record<string, string> = {
  postgres: '5432',
  mysql: '3306',
  mssql: '1433',
  oracle: '1521',
};

const DSN_PLACEHOLDERS: Record<string, string> = {
  postgres: 'postgres://user:pass@localhost:5432/dbname?sslmode=disable',
  mysql: 'user:pass@tcp(localhost:3306)/dbname',
  mssql: 'sqlserver://user:pass@localhost:1433?database=dbname',
  snowflake: 'user:pass@account/dbname/schema?warehouse=wh',
};

const DSN_HELP: Record<string, string> = {
  mysql: 'Format: user:pass@tcp(host:port)/dbname. The tcp() wrapper is required.',
  postgres: 'Format: postgres://user:pass@host:port/dbname?sslmode=disable',
  mssql: 'Format: sqlserver://user:pass@host:port?database=dbname',
};

const PARAMS_PLACEHOLDERS: Record<string, string> = {
  postgres: 'sslmode=disable',
  mysql: 'parseTime=true',
  mssql: 'encrypt=disable',
  snowflake: 'warehouse=COMPUTE_WH&role=ANALYST',
};

/** True when the value has enough input to submit. */
export function connectionReady(v: ConnectionValue): boolean {
  return v.mode === 'dsn' ? v.dsn.trim() !== '' : v.host.trim() !== '';
}

/** Parse "a=1&b=2" or one pair per line into an object. */
function parseParams(raw: string): Record<string, string> | undefined {
  const out: Record<string, string> = {};
  for (const part of raw.split(/[&\n]/)) {
    const trimmed = part.trim();
    if (!trimmed) continue;
    const eq = trimmed.indexOf('=');
    if (eq <= 0) {
      throw new Error(`Invalid option "${trimmed}". Use key=value.`);
    }
    out[trimmed.slice(0, eq).trim()] = trimmed.slice(eq + 1).trim();
  }
  return Object.keys(out).length ? out : undefined;
}

/**
 * Request body fields for the service API: either { dsn } or
 * { connection: {...} }. Throws on invalid input so callers can show it.
 */
export function connectionPayload(v: ConnectionValue): Record<string, any> {
  if (v.mode === 'dsn') {
    return { dsn: v.dsn.trim() };
  }
  const conn: Record<string, any> = { host: v.host.trim() };
  if (v.port.trim()) {
    const port = Number(v.port.trim());
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
      throw new Error('Port must be a number between 1 and 65535');
    }
    conn.port = port;
  }
  if (v.user) conn.user = v.user.trim();
  if (v.password) conn.password = v.password; // passwords are not trimmed
  if (v.database.trim()) conn.database = v.database.trim();
  const params = parseParams(v.params);
  if (params) conn.params = params;
  return { connection: conn };
}

interface Props {
  driver: string;
  value: ConnectionValue;
  onChange: (v: ConnectionValue) => void;
}

export function ConnectionInput({ driver, value, onChange }: Props) {
  const [showPassword, setShowPassword] = useState(false);
  const set = (patch: Partial<ConnectionValue>) => onChange({ ...value, ...patch });
  const input = (key: keyof ConnectionValue) => (e: Event) =>
    set({ [key]: (e.target as HTMLInputElement).value } as Partial<ConnectionValue>);

  const isSnowflake = driver === 'snowflake';
  const isOracle = driver === 'oracle';
  const label = 'block text-sm font-medium text-text-secondary mb-1.5';
  const optional = <span class="text-text-muted font-normal">(optional)</span>;

  const modeButton = (mode: ConnectionMode, text: string) => (
    <button
      type="button"
      onClick={() => set({ mode })}
      aria-pressed={value.mode === mode}
      class={`px-3 py-1.5 text-xs font-medium rounded-md transition-colors ${
        value.mode === mode
          ? 'bg-brand/10 text-brand'
          : 'text-text-secondary hover:text-text-primary'
      }`}
    >
      {text}
    </button>
  );

  return (
    <div class="space-y-4">
      <div class="flex items-center justify-between">
        <span class="text-sm font-medium text-text-secondary">Connection</span>
        <div class="inline-flex gap-1 p-0.5 rounded-lg border border-border-default">
          {modeButton('fields', 'Individual fields')}
          {modeButton('dsn', 'Connection string')}
        </div>
      </div>

      {value.mode === 'dsn' ? (
        <div>
          <label class={label}>DSN (Connection String)</label>
          <input
            type="text"
            class="input w-full font-mono text-sm"
            placeholder={DSN_PLACEHOLDERS[driver] || ''}
            value={value.dsn}
            onInput={input('dsn')}
            autoComplete="off"
            spellcheck={false}
          />
          <p class="text-xs text-text-muted mt-1">
            {DSN_HELP[driver] || 'Full connection string for the database'}
          </p>
        </div>
      ) : (
        <div class="space-y-4">
          <div class="grid grid-cols-3 gap-3">
            <div class={isSnowflake ? 'col-span-3' : 'col-span-2'}>
              <label class={label}>{isSnowflake ? 'Account' : 'Host'}</label>
              <input
                type="text"
                class="input w-full font-mono text-sm"
                placeholder={isSnowflake ? 'myorg-myaccount' : 'localhost'}
                value={value.host}
                onInput={input('host')}
                autoComplete="off"
                spellcheck={false}
              />
            </div>
            {!isSnowflake && (
              <div>
                <label class={label}>Port</label>
                <input
                  type="text"
                  inputMode="numeric"
                  class="input w-full font-mono text-sm"
                  placeholder={DEFAULT_PORTS[driver] || ''}
                  value={value.port}
                  onInput={input('port')}
                />
              </div>
            )}
          </div>

          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class={label}>Username</label>
              <input
                type="text"
                class="input w-full font-mono text-sm"
                value={value.user}
                onInput={input('user')}
                autoComplete="off"
                spellcheck={false}
              />
            </div>
            <div>
              <label class={label}>Password</label>
              <div class="relative">
                <input
                  type={showPassword ? 'text' : 'password'}
                  class="input w-full font-mono text-sm pr-14"
                  value={value.password}
                  onInput={input('password')}
                  autoComplete="new-password"
                />
                <button
                  type="button"
                  onClick={() => setShowPassword(!showPassword)}
                  class="absolute inset-y-0 right-2 text-xs text-text-muted hover:text-text-primary"
                >
                  {showPassword ? 'Hide' : 'Show'}
                </button>
              </div>
            </div>
          </div>

          <div>
            <label class={label}>
              {isOracle ? 'Service Name' : 'Database'} {!isOracle && optional}
            </label>
            <input
              type="text"
              class="input w-full font-mono text-sm"
              placeholder={isSnowflake ? 'MY_DB or MY_DB/MY_SCHEMA' : isOracle ? 'XEPDB1' : 'dbname'}
              value={value.database}
              onInput={input('database')}
              autoComplete="off"
              spellcheck={false}
            />
          </div>

          <div>
            <label class={label}>Options {optional}</label>
            <input
              type="text"
              class="input w-full font-mono text-sm"
              placeholder={PARAMS_PLACEHOLDERS[driver] || 'key=value&key2=value2'}
              value={value.params}
              onInput={input('params')}
              autoComplete="off"
              spellcheck={false}
            />
            <p class="text-xs text-text-muted mt-1">
              Extra driver options as key=value, separated by &amp;. Special characters in the password are escaped for you.
            </p>
          </div>
        </div>
      )}
    </div>
  );
}
