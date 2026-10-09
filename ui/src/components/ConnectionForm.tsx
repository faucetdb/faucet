import { useMemo, useRef, useState } from 'preact/hooks';
import { apiFetch, errorMessage } from '../hooks/useApi';
import {
  API_NAME_PATTERN,
  ConnectionParams,
  ENGINES,
  EngineId,
  describeConnection,
  engineById,
  slugify,
} from '../lib/drivers';
import { DbLogo } from './DbLogo';
import { Icon } from './Icon';
import { Field, Input, PasswordInput, Segmented, Select, Switch, Textarea, useId } from './ui';

export interface DbForm {
  engine: EngineId;
  mode: 'fields' | 'dsn';
  name: string;
  /** Once the user edits the API name we stop deriving it from the database. */
  nameEdited: boolean;
  host: string;
  port: string;
  database: string;
  username: string;
  password: string;
  sslMode: string;
  trustCert: boolean;
  path: string;
  account: string;
  warehouse: string;
  role: string;
  authMethod: 'password' | 'keypair';
  privateKeyPath: string;
  dsn: string;
  schema: string;
  readOnly: boolean;
  rawSql: boolean;
}

export function emptyDbForm(engine: EngineId = 'postgres'): DbForm {
  return {
    engine,
    mode: 'fields',
    name: '',
    nameEdited: false,
    host: engine === 'snowflake' || engine === 'sqlite' ? '' : 'localhost',
    port: '',
    database: '',
    username: '',
    password: '',
    sslMode: '',
    trustCert: false,
    path: '',
    account: '',
    warehouse: '',
    role: '',
    authMethod: 'password',
    privateKeyPath: '',
    dsn: '',
    schema: '',
    readOnly: false,
    rawSql: false,
  };
}

/** Existing service as returned by GET /api/v1/system/service. */
export interface ServiceRecord {
  id: number;
  name: string;
  label?: string;
  driver: string;
  schema?: string;
  read_only: boolean;
  raw_sql_allowed: boolean;
  is_active: boolean;
  schema_lock?: string;
  private_key_path?: string;
  connection?: ConnectionParams;
  created_at?: string;
}

/** Pre-fill the form from a saved service (never includes the password). */
export function dbFormFromService(svc: ServiceRecord): DbForm {
  const c = svc.connection || {};
  const engine: EngineId = (ENGINES.find((e) => e.driver === svc.driver)?.id || 'postgres') as EngineId;
  return {
    ...emptyDbForm(engine),
    name: svc.name,
    nameEdited: true,
    host: c.host || '',
    port: c.port ? String(c.port) : '',
    database: c.database || '',
    username: c.username || '',
    sslMode: c.ssl_mode || '',
    trustCert: c.options?.TrustServerCertificate === 'true',
    path: c.path || '',
    account: c.account || '',
    warehouse: c.warehouse || '',
    role: c.role || '',
    authMethod: svc.private_key_path ? 'keypair' : 'password',
    privateKeyPath: svc.private_key_path || '',
    schema: svc.driver === 'snowflake' ? c.schema || svc.schema || '' : svc.schema || '',
    readOnly: svc.read_only,
    rawSql: svc.raw_sql_allowed,
  };
}

export type FormErrors = Partial<Record<'name' | 'host' | 'port' | 'database' | 'path' | 'account' | 'username' | 'dsn' | 'privateKeyPath', string>>;

export function validateDbForm(f: DbForm, editing: boolean): FormErrors {
  const e: FormErrors = {};
  const engine = engineById(f.engine);
  if (!editing) {
    if (!f.name) e.name = 'Give this database an API name.';
    else if (!API_NAME_PATTERN.test(f.name)) e.name = 'Use letters, numbers, dashes and underscores only.';
  }
  if (f.mode === 'dsn') {
    if (!f.dsn.trim() && !editing) e.dsn = 'Paste a connection string.';
    return e;
  }
  if (engine.kind === 'network') {
    if (!f.host.trim()) e.host = 'Enter the server host name or IP address.';
    if (f.port && !/^\d+$/.test(f.port)) e.port = 'Ports are numbers.';
    else if (f.port && (+f.port < 1 || +f.port > 65535)) e.port = 'Use a port between 1 and 65535.';
    if (f.engine === 'oracle' && !f.database.trim()) e.database = 'Oracle needs a service name.';
  } else if (engine.kind === 'file') {
    if (!f.path.trim()) e.path = 'Enter the path to the SQLite file on the Faucet server.';
  } else {
    if (!f.account.trim()) e.account = 'Enter your Snowflake account identifier.';
    if (!f.username.trim()) e.username = 'Enter the Snowflake user.';
    if (f.authMethod === 'keypair' && !f.privateKeyPath.trim()) e.privateKeyPath = 'Enter the path to the private key file.';
  }
  return e;
}

/** Request body for create, update and connection test endpoints. */
export function dbFormToRequest(f: DbForm) {
  const engine = engineById(f.engine);
  const body: Record<string, unknown> = {
    name: f.name.trim(),
    driver: engine.driver,
    read_only: f.readOnly,
    raw_sql_allowed: f.rawSql,
  };
  if (f.schema.trim()) body.schema = f.schema.trim();
  if (engine.kind === 'snowflake' && f.authMethod === 'keypair' && f.privateKeyPath.trim()) {
    body.private_key_path = f.privateKeyPath.trim();
  }
  if (f.mode === 'dsn') {
    if (f.dsn.trim()) body.dsn = f.dsn.trim();
    return body;
  }
  const c: ConnectionParams = {};
  if (engine.kind === 'network') {
    c.host = f.host.trim();
    if (f.port) c.port = parseInt(f.port, 10);
    if (f.database.trim()) c.database = f.database.trim();
    if (f.username.trim()) c.username = f.username.trim();
    if (f.password) c.password = f.password;
    if (f.sslMode) c.ssl_mode = f.sslMode;
    if (f.engine === 'mssql' && f.trustCert) c.options = { TrustServerCertificate: 'true' };
  } else if (engine.kind === 'file') {
    c.path = f.path.trim();
  } else {
    c.account = f.account.trim();
    c.username = f.username.trim();
    if (f.authMethod === 'password' && f.password) c.password = f.password;
    if (f.database.trim()) c.database = f.database.trim();
    if (f.schema.trim()) c.schema = f.schema.trim();
    if (f.warehouse.trim()) c.warehouse = f.warehouse.trim();
    if (f.role.trim()) c.role = f.role.trim();
  }
  body.connection = c;
  return body;
}

/** "Where is it" summary for the flow line while the form is being filled. */
export function dbFormSource(f: DbForm): string {
  const engine = engineById(f.engine);
  if (f.mode === 'dsn') return f.dsn ? 'connection string' : '';
  if (engine.kind === 'file') return f.path.trim().split(/[\\/]/).pop() || '';
  return describeConnection(engine.driver, {
    host: f.host,
    port: f.port ? +f.port : engine.defaultPort,
    database: f.database,
    path: f.path,
    account: f.account,
    schema: f.schema,
  });
}

export interface ProbeResult {
  ok: boolean;
  message: string;
  tableCount?: number;
  latencyMs?: number;
  /** Serialized request the result belongs to, to detect stale results. */
  key: string;
}

/** Try the settings without saving. `existingName` reuses a stored password. */
export async function probeConnection(f: DbForm, existingName?: string): Promise<ProbeResult> {
  const body = dbFormToRequest(f);
  const key = JSON.stringify(body);
  if (existingName) body.name = existingName;
  try {
    const res = await apiFetch('/api/v1/system/connection/test', { method: 'POST', body });
    const n = res.table_count ?? 0;
    return {
      ok: true,
      key,
      tableCount: n,
      latencyMs: res.latency_ms,
      message: n === 0 ? 'Connected. No tables found yet; check the schema setting if you expected some.' : `Connected. Found ${n} ${n === 1 ? 'table' : 'tables'}.`,
    };
  } catch (err) {
    return { ok: false, key, message: errorMessage(err, 'Could not connect') };
  }
}

export function probeKey(f: DbForm): string {
  return JSON.stringify(dbFormToRequest(f));
}

/* -------------------------------------------------------------- Component */

export function EnginePicker({ value, onChange }: { value: EngineId; onChange: (e: EngineId) => void }) {
  return (
    <div role="radiogroup" aria-label="Database type" class="grid grid-cols-2 sm:grid-cols-4 gap-2">
      {ENGINES.map((e) => {
        const selected = value === e.id;
        return (
          <button
            key={e.id}
            type="button"
            role="radio"
            aria-checked={selected}
            onClick={() => onChange(e.id)}
            class={`flex items-center gap-2.5 h-12 px-2.5 rounded-[8px] border text-left text-sm font-medium transition-colors ${
              selected ? 'border-brand bg-brand/8 text-fg ring-1 ring-brand' : 'border-line bg-panel text-fg-muted hover:text-fg hover:border-line-strong'
            }`}
          >
            <DbLogo engine={e.id} size={28} />
            <span class="truncate">{e.label}</span>
          </button>
        );
      })}
    </div>
  );
}

export function ConnectionForm({ form, onChange, errors, editing = false, showAdvanced = true }: {
  form: DbForm;
  onChange: (f: DbForm) => void;
  errors: FormErrors;
  editing?: boolean;
  showAdvanced?: boolean;
}) {
  const engine = engineById(form.engine);
  const [advancedOpen, setAdvancedOpen] = useState(form.readOnly || form.rawSql || (!!form.schema && engine.kind !== 'snowflake'));
  const [pastedHint, setPastedHint] = useState(false);
  const ids = {
    name: useId('name'), host: useId('host'), port: useId('port'), db: useId('db'), user: useId('user'), pass: useId('pass'),
    ssl: useId('ssl'), path: useId('path'), account: useId('acct'), wh: useId('wh'), role: useId('role'), schema: useId('schema'),
    key: useId('key'), dsn: useId('dsn'),
  };
  const formRef = useRef(form);
  formRef.current = form;

  /** Update fields; keep the API name in sync with the database name until edited. */
  const set = (patch: Partial<DbForm>) => {
    const next = { ...formRef.current, ...patch };
    if (!next.nameEdited && !editing) {
      const source = engine.kind === 'file' ? next.path : next.database;
      next.name = slugify(source || '');
    }
    onChange(next);
  };

  const pickEngine = (id: EngineId) => {
    const fresh = emptyDbForm(id);
    // Keep what carries over between engines.
    onChange({
      ...fresh,
      name: form.name,
      nameEdited: form.nameEdited,
      host: engineById(id).kind === 'network' ? form.host || fresh.host : '',
      database: engineById(id).kind === 'file' ? '' : form.database,
      username: form.username,
      password: form.password,
      readOnly: form.readOnly,
      rawSql: form.rawSql,
    });
  };

  // Pasting a full URL into Host is common; switch to connection-string mode.
  const onHost = (v: string) => {
    if (/:\/\/|@tcp\(/.test(v)) {
      set({ mode: 'dsn', dsn: v.trim(), host: '' });
      setPastedHint(true);
      return;
    }
    set({ host: v });
  };

  const passwordHint = editing ? 'Leave blank to keep the saved password.' : 'Special characters are fine; Faucet escapes them for you.';

  return (
    <div class="flex flex-col gap-6">
      {!editing ? (
        <section class="flex flex-col gap-2.5">
          <h3 class="text-sm font-medium text-fg">Database type</h3>
          <EnginePicker value={form.engine} onChange={pickEngine} />
          {form.engine === 'mariadb' && (
            <p class="text-xs text-fg-muted">MariaDB uses Faucet's MySQL driver; it will be listed as MySQL.</p>
          )}
        </section>
      ) : (
        <div class="flex items-center gap-3">
          <DbLogo engine={form.engine} size={32} />
          <div>
            <p class="text-sm font-medium text-fg">{engine.label}</p>
            <p class="text-xs text-fg-muted">The database type can't be changed. Add a new database instead.</p>
          </div>
        </div>
      )}

      <section class="flex flex-col gap-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <h3 class="text-sm font-medium text-fg">Connection</h3>
          {engine.kind !== 'file' && (
            <Segmented
              label="How to enter the connection"
              value={form.mode}
              onChange={(mode) => {
                setPastedHint(false);
                set({ mode });
              }}
              options={[
                { value: 'fields', label: 'Fields' },
                { value: 'dsn', label: 'Connection string' },
              ]}
            />
          )}
        </div>

        {form.mode === 'dsn' && engine.kind !== 'file' ? (
          <Field
            label="Connection string"
            htmlFor={ids.dsn}
            error={errors.dsn}
            hint={
              pastedHint ? (
                <span>That looked like a full connection string, so it's here instead. Switch back to Fields to enter parts.</span>
              ) : editing ? (
                'Leave blank to keep the saved connection string.'
              ) : (
                <span>For example <span class="font-mono">{engine.dsnExample}</span></span>
              )
            }
          >
            <Textarea
              id={ids.dsn}
              mono
              rows={3}
              value={form.dsn}
              onValue={(dsn) => set({ dsn })}
              placeholder={editing ? 'Saved connection string is kept' : engine.dsnExample}
              spellcheck={false}
              autocomplete="off"
            />
          </Field>
        ) : engine.kind === 'network' ? (
          <>
            <div class="grid grid-cols-[1fr_7rem] gap-3">
              <Field label="Host" htmlFor={ids.host} error={errors.host}>
                <Input id={ids.host} value={form.host} onValue={onHost} placeholder="db.example.com" invalid={!!errors.host} autocomplete="off" spellcheck={false} />
              </Field>
              <Field label="Port" htmlFor={ids.port} error={errors.port}>
                <Input id={ids.port} value={form.port} onValue={(port) => set({ port })} placeholder={String(engine.defaultPort)} inputMode="numeric" invalid={!!errors.port} />
              </Field>
            </div>
            <Field label={engine.databaseLabel} htmlFor={ids.db} error={errors.database} optional={form.engine !== 'oracle'}>
              <Input id={ids.db} value={form.database} onValue={(database) => set({ database })} placeholder={engine.databasePlaceholder} invalid={!!errors.database} autocomplete="off" spellcheck={false} />
            </Field>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <Field label="Username" htmlFor={ids.user}>
                <Input id={ids.user} value={form.username} onValue={(username) => set({ username })} placeholder={engine.userPlaceholder} autocomplete="off" spellcheck={false} />
              </Field>
              <Field label="Password" htmlFor={ids.pass} hint={passwordHint}>
                <PasswordInput id={ids.pass} value={form.password} onValue={(password) => set({ password })} placeholder={editing ? 'Unchanged' : ''} autocomplete="new-password" />
              </Field>
            </div>
            {engine.tls && (
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 items-start">
                <Field label={engine.tls.label} htmlFor={ids.ssl} hint={engine.tls.hint}>
                  <Select id={ids.ssl} value={form.sslMode} onValue={(sslMode) => set({ sslMode })} options={engine.tls.options} />
                </Field>
                {form.engine === 'mssql' && (
                  <div class="sm:pt-6">
                    <Switch
                      checked={form.trustCert}
                      onChange={(trustCert) => set({ trustCert })}
                      label="Trust server certificate"
                      description="For self-signed certificates in development."
                    />
                  </div>
                )}
              </div>
            )}
          </>
        ) : engine.kind === 'file' ? (
          <Field
            label="File path"
            htmlFor={ids.path}
            error={errors.path}
            hint="The path on the machine running Faucet, not your browser. Relative paths start from where Faucet was launched."
          >
            <Input id={ids.path} mono value={form.path} onValue={(path) => set({ path })} placeholder="/data/app.db" invalid={!!errors.path} spellcheck={false} />
          </Field>
        ) : (
          <>
            <Field label="Account identifier" htmlFor={ids.account} error={errors.account} hint={<span>Shaped like <span class="font-mono">orgname-accountname</span>. Find it under Admin, then Accounts in Snowsight.</span>}>
              <Input id={ids.account} mono value={form.account} onValue={(account) => set({ account })} placeholder="myorg-myaccount" invalid={!!errors.account} spellcheck={false} />
            </Field>
            <Field label="Username" htmlFor={ids.user} error={errors.username}>
              <Input id={ids.user} value={form.username} onValue={(username) => set({ username })} placeholder={engine.userPlaceholder} invalid={!!errors.username} spellcheck={false} />
            </Field>
            <div class="flex flex-col gap-3">
              <Segmented
                label="Authentication"
                value={form.authMethod}
                onChange={(authMethod) => set({ authMethod })}
                options={[
                  { value: 'password', label: 'Password' },
                  { value: 'keypair', label: 'Key pair' },
                ]}
              />
              {form.authMethod === 'password' ? (
                <Field label="Password" htmlFor={ids.pass} hint={passwordHint}>
                  <PasswordInput id={ids.pass} value={form.password} onValue={(password) => set({ password })} placeholder={editing ? 'Unchanged' : ''} autocomplete="new-password" />
                </Field>
              ) : (
                <Field label="Private key file" htmlFor={ids.key} error={errors.privateKeyPath} hint="Path to an unencrypted PKCS#8 .p8 file on the Faucet server.">
                  <Input id={ids.key} mono value={form.privateKeyPath} onValue={(privateKeyPath) => set({ privateKeyPath })} placeholder="/secrets/rsa_key.p8" invalid={!!errors.privateKeyPath} spellcheck={false} />
                </Field>
              )}
            </div>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <Field label="Database" htmlFor={ids.db}>
                <Input id={ids.db} value={form.database} onValue={(database) => set({ database })} placeholder="ANALYTICS" spellcheck={false} />
              </Field>
              <Field label="Schema" htmlFor={ids.schema}>
                <Input id={ids.schema} value={form.schema} onValue={(schema) => set({ schema })} placeholder="PUBLIC" spellcheck={false} />
              </Field>
              <Field label="Warehouse" htmlFor={ids.wh}>
                <Input id={ids.wh} value={form.warehouse} onValue={(warehouse) => set({ warehouse })} placeholder="COMPUTE_WH" spellcheck={false} />
              </Field>
              <Field label="Role" htmlFor={ids.role} optional>
                <Input id={ids.role} value={form.role} onValue={(role) => set({ role })} placeholder="FAUCET_READER" spellcheck={false} />
              </Field>
            </div>
          </>
        )}
      </section>

      <section class="flex flex-col gap-4">
        <Field
          label="API name"
          htmlFor={ids.name}
          error={errors.name}
          hint={
            editing ? (
              <span>API names can't be changed because clients already call <span class="font-mono">/api/v1/{form.name}</span>.</span>
            ) : (
              <span>Becomes part of every URL: <span class="font-mono text-fg">/api/v1/{form.name || 'name'}/_table/…</span></span>
            )
          }
        >
          <Input
            id={ids.name}
            mono
            value={form.name}
            disabled={editing}
            onValue={(name) => onChange({ ...form, name, nameEdited: name !== '' })}
            placeholder="shop"
            invalid={!!errors.name}
            spellcheck={false}
            autocomplete="off"
          />
        </Field>
      </section>

      {showAdvanced && (
        <section class="border-t border-line pt-4">
          <button
            type="button"
            class="flex items-center gap-1.5 text-sm font-medium text-fg-muted hover:text-fg"
            aria-expanded={advancedOpen}
            onClick={() => setAdvancedOpen(!advancedOpen)}
          >
            <Icon name={advancedOpen ? 'chevronDown' : 'chevronRight'} size={14} />
            Access and schema options
          </button>
          {advancedOpen && (
            <div class="flex flex-col gap-4 mt-4">
              <Switch
                checked={form.readOnly}
                onChange={(readOnly) => set({ readOnly })}
                label="Read-only"
                description="Block inserts, updates and deletes through the REST API and MCP, whatever a role allows."
              />
              <Switch
                checked={form.rawSql}
                onChange={(rawSql) => set({ rawSql })}
                label="Allow raw SQL for AI agents"
                description="Lets MCP clients run arbitrary SQL with faucet_raw_sql. Leave off unless you trust every key holder."
              />
              {engine.kind !== 'snowflake' && (
                <Field label="Schema" htmlFor={ids.schema} optional hint={engine.defaultSchema ? `Defaults to ${engine.defaultSchema}.` : 'Defaults to the database you connect to.'}>
                  <Input id={ids.schema} value={form.schema} onValue={(schema) => set({ schema })} placeholder={engine.defaultSchema || ''} spellcheck={false} />
                </Field>
              )}
            </div>
          )}
        </section>
      )}
    </div>
  );
}

/** Memoized request key so callers can tell whether a test result is stale. */
export function useProbeKey(form: DbForm) {
  return useMemo(() => probeKey(form), [form]);
}
