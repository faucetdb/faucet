import { useEffect, useState } from 'preact/hooks';
import { DbLogo } from '../components/DbLogo';
import { Icon, IconName } from '../components/Icon';
import { Modal, toast } from '../components/Overlay';
import { Button, CopyButton, Field, Input, Notice, PageHeader, Panel, PasswordInput, Segmented, Skeleton, Tag, useId } from '../components/ui';
import { apiFetch, errorMessage } from '../hooks/useApi';
import { ENGINES } from '../lib/drivers';
import { formatDate, serverOrigin, timeAgo } from '../lib/format';
import { DOCS_URL, GITHUB_URL, useServerInfo } from '../lib/server';
import { ThemeChoice, getTheme, setTheme } from '../lib/theme';

/* Shape of adminToMap in internal/handler/system.go. */
interface Admin {
  id: number;
  email: string;
  name: string;
  is_active: boolean;
  is_super_admin: boolean;
  created_at: string;
  updated_at: string;
  last_login_at?: string;
}

const TELEMETRY_URL = `${GITHUB_URL}/blob/main/TELEMETRY.md`;

export function Settings() {
  return (
    <div>
      <PageHeader title="Settings" description="This Faucet server, who can sign in to it, and how it looks." />
      <div class="flex flex-col gap-6 max-w-[860px]">
        <ServerPanel />
        <AdminsPanel />
        <AppearancePanel />
        <TelemetryPanel />
        <HelpPanel />
      </div>
    </div>
  );
}

/* ---------------------------------------------------------------- Server */

function ServerPanel() {
  const info = useServerInfo();
  const origin = serverOrigin();
  const drivers = info?.drivers || [];
  // One entry per engine the server can talk to; MariaDB rides on mysql.
  const engines = ENGINES.filter((e) => drivers.includes(e.driver));
  const unknown = drivers.filter((d) => !ENGINES.some((e) => e.driver === d));

  const urls = [
    { label: 'REST API', value: `${origin}/api/v1/{database}`, copy: `${origin}/api/v1`, hint: 'One API per database, named after it.' },
    { label: 'MCP server', value: `${origin}/mcp`, hint: 'For AI agents. Send an API key in X-API-Key.' },
    { label: 'OpenAPI spec', value: `${origin}/openapi.json`, hint: 'All databases in one spec. No key needed.' },
    { label: 'Health', value: `${origin}/healthz`, hint: 'Returns 200 while the process is running.' },
    { label: 'Readiness', value: `${origin}/readyz`, hint: 'Returns 503 when any database is unreachable.' },
  ];

  return (
    <Panel title="Server">
      <dl class="divide-y divide-line">
        <div class="flex flex-wrap items-center gap-x-4 gap-y-1 px-5 py-3">
          <dt class="w-36 shrink-0 text-sm text-fg-muted">Version</dt>
          <dd class="text-sm text-fg">
            {info === null ? <Skeleton class="h-4 w-20" /> : info.version ? <span class="font-mono">{info.version}</span> : 'Unknown'}
          </dd>
        </div>
        <div class="flex flex-wrap items-start gap-x-4 gap-y-2 px-5 py-3">
          <dt class="w-36 shrink-0 text-sm text-fg-muted pt-1">Databases supported</dt>
          <dd class="flex-1 min-w-0">
            {info === null ? (
              <Skeleton class="h-6 w-64 max-w-full" />
            ) : drivers.length === 0 ? (
              <span class="text-sm text-fg-muted">Could not read the driver list.</span>
            ) : (
              <>
                <ul class="flex flex-wrap gap-x-4 gap-y-2">
                  {engines.map((e) => (
                    <li key={e.id} class="inline-flex items-center gap-2 text-sm text-fg">
                      <DbLogo engine={e.id} size={22} />
                      {e.label}
                    </li>
                  ))}
                  {unknown.map((d) => (
                    <li key={d} class="inline-flex items-center gap-2 text-sm text-fg font-mono">{d}</li>
                  ))}
                </ul>
                {drivers.includes('mysql') && <p class="text-xs text-fg-muted mt-2">MariaDB connects through the MySQL driver.</p>}
              </>
            )}
          </dd>
        </div>
        {urls.map((u) => (
          <div key={u.label} class="flex flex-wrap items-start gap-x-4 gap-y-1 px-5 py-3">
            <dt class="w-36 shrink-0 text-sm text-fg-muted pt-1">{u.label}</dt>
            <dd class="flex-1 min-w-0 flex items-start gap-2">
              <div class="flex-1 min-w-0">
                <code class="block text-[12.5px] text-fg break-all pt-1">{u.value}</code>
                <p class="text-xs text-fg-muted mt-0.5">{u.hint}</p>
              </div>
              <CopyButton text={u.copy || u.value} />
            </dd>
          </div>
        ))}
      </dl>
    </Panel>
  );
}

/* ---------------------------------------------------------------- Admins */

function AdminsPanel() {
  const [admins, setAdmins] = useState<Admin[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);
  const [flash, setFlash] = useState<string | null>(null);

  useEffect(() => {
    load();
  }, []);

  async function load() {
    try {
      const res = await apiFetch<{ resource: Admin[] }>('/api/v1/system/admin');
      setAdmins(res.resource || []);
      setLoadError(null);
    } catch (err) {
      setLoadError(errorMessage(err));
      setAdmins([]);
    }
  }

  return (
    <Panel
      title="Admins"
      description="People who can sign in to this console. Apps and agents use API keys instead."
      actions={
        <Button size="sm" icon="plus" onClick={() => setAdding(true)}>
          Add admin
        </Button>
      }
    >
      {loadError && (
        <div class="px-5 py-3">
          <Notice tone="bad" title="Could not load admins">
            {loadError}
          </Notice>
        </div>
      )}
      {admins === null ? (
        <div class="divide-y divide-line">
          {[0, 1].map((i) => (
            <div key={i} class="flex items-center gap-3 px-5 py-3">
              <Skeleton class="w-8 h-8 rounded-full" />
              <div class="flex-1 flex flex-col gap-1.5">
                <Skeleton class="h-3.5 w-40" />
                <Skeleton class="h-3 w-56" />
              </div>
            </div>
          ))}
        </div>
      ) : (
        <ul class="divide-y divide-line">
          {admins.map((a) => (
            <li key={a.id} class={`flex flex-wrap items-center gap-x-3 gap-y-1.5 px-5 py-3 ${flash === a.email ? 'anim-flash' : ''}`}>
              <span
                class="inline-flex items-center justify-center w-8 h-8 rounded-full bg-brand/12 text-brand-fg text-sm font-semibold shrink-0"
                aria-hidden="true"
              >
                {(a.name || a.email).trim().charAt(0).toUpperCase()}
              </span>
              <div class="min-w-0 flex-1 basis-48">
                <div class="flex flex-wrap items-center gap-2">
                  <p class="text-sm font-medium text-fg truncate">{a.name || a.email}</p>
                  {a.is_super_admin && <Tag tone="brand">Owner</Tag>}
                  {!a.is_active && <Tag tone="warn">Disabled</Tag>}
                </div>
                {a.name && <p class="text-xs text-fg-muted truncate">{a.email}</p>}
              </div>
              <div class="text-xs text-fg-muted sm:text-right">
                <p>{a.last_login_at ? `Signed in ${timeAgo(a.last_login_at).toLowerCase()}` : 'Never signed in'}</p>
                {a.created_at && <p class="text-fg-faint">Added {formatDate(a.created_at)}</p>}
              </div>
            </li>
          ))}
        </ul>
      )}
      <p class="px-5 py-3 border-t border-line text-xs text-fg-muted">
        Every admin has full access. Admins can't be removed from the console yet; list them with{' '}
        <code class="code-chip">faucet admin list</code>.
      </p>

      <AddAdminModal
        open={adding}
        onClose={() => setAdding(false)}
        onAdded={(email) => {
          setAdding(false);
          setFlash(email);
          toast(`Admin added. ${email} can sign in now`);
          load();
        }}
      />
    </Panel>
  );
}

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

function AddAdminModal({ open, onClose, onAdded }: { open: boolean; onClose: () => void; onAdded: (email: string) => void }) {
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [errors, setErrors] = useState<{ email?: string; password?: string }>({});
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const nameId = useId('adm-name');
  const emailId = useId('adm-email');
  const pwId = useId('adm-pw');

  useEffect(() => {
    if (!open) return;
    setName('');
    setEmail('');
    setPassword('');
    setErrors({});
    setSaveError(null);
  }, [open]);

  function validate() {
    const e: typeof errors = {};
    if (!email.trim()) e.email = 'Enter an email address.';
    else if (!EMAIL_PATTERN.test(email.trim())) e.email = 'Enter a valid email address, like ana@example.com.';
    if (!password) e.password = 'Enter a password.';
    else if (password.length < 8) e.password = 'Use at least 8 characters.';
    return e;
  }

  async function submit(ev?: Event) {
    ev?.preventDefault();
    const e = validate();
    setErrors(e);
    if (Object.keys(e).length) return;
    setSaving(true);
    setSaveError(null);
    try {
      await apiFetch('/api/v1/system/admin', {
        method: 'POST',
        body: { email: email.trim(), password, name: name.trim() },
      });
      onAdded(email.trim());
    } catch (err) {
      setSaveError(errorMessage(err, 'The admin could not be added.'));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Add admin"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>Cancel</Button>
          <Button variant="primary" type="submit" form="add-admin-form" loading={saving}>
            Add admin
          </Button>
        </>
      }
    >
      <form id="add-admin-form" class="flex flex-col gap-4" onSubmit={submit} noValidate>
        <p class="text-sm text-fg-muted">They sign in with this email and password and get full admin access. Share the password with them directly.</p>
        {saveError && <Notice tone="bad" title="Not added">{saveError}</Notice>}
        <Field label="Name" htmlFor={nameId} optional>
          <Input id={nameId} value={name} onValue={setName} autoComplete="off" placeholder="Ana Lopez" />
        </Field>
        <Field label="Email" htmlFor={emailId} error={errors.email}>
          <Input
            id={emailId}
            type="email"
            value={email}
            onValue={(v) => {
              setEmail(v);
              if (errors.email) setErrors({ ...errors, email: undefined });
            }}
            invalid={!!errors.email}
            autoComplete="off"
            placeholder="ana@example.com"
          />
        </Field>
        <Field label="Password" htmlFor={pwId} error={errors.password} hint="At least 8 characters.">
          <PasswordInput
            id={pwId}
            value={password}
            onValue={(v) => {
              setPassword(v);
              if (errors.password) setErrors({ ...errors, password: undefined });
            }}
            invalid={!!errors.password}
            autoComplete="new-password"
          />
        </Field>
      </form>
    </Modal>
  );
}

/* ------------------------------------------------------------ Appearance */

function AppearancePanel() {
  const [theme, setChoice] = useState<ThemeChoice>(getTheme());
  return (
    <Panel title="Appearance">
      <div class="flex flex-wrap items-center justify-between gap-3 px-5 py-4">
        <div>
          <p class="text-sm font-medium text-fg">Theme</p>
          <p class="text-xs text-fg-muted mt-0.5">System follows your operating system. Saved in this browser only.</p>
        </div>
        <Segmented<ThemeChoice>
          label="Theme"
          value={theme}
          onChange={(t) => {
            setTheme(t);
            setChoice(t);
          }}
          options={[
            { value: 'system', label: 'System' },
            { value: 'light', label: 'Light' },
            { value: 'dark', label: 'Dark' },
          ]}
        />
      </div>
    </Panel>
  );
}

/* ------------------------------------------------------------- Telemetry */

function TelemetryPanel() {
  return (
    <Panel title="Usage data">
      <div class="px-5 py-4 flex flex-col gap-3 text-sm text-fg-muted max-w-[72ch]">
        <p>
          Faucet sends an anonymous usage report once at startup and then hourly: version, operating system, which database types are in use, and
          counts of databases, tables, keys and roles. It never sends connection details, credentials, table or column names, data, queries or IP
          addresses. Turning it off changes nothing else.
        </p>
        <p>To turn it off, set this environment variable for the Faucet process, or run the CLI command, then restart Faucet:</p>
        <div class="flex flex-col gap-2">
          <CommandLine command="FAUCET_TELEMETRY=0" />
          <CommandLine command="faucet config set telemetry.enabled false" />
        </div>
        <p>
          <a href={TELEMETRY_URL} target="_blank" rel="noopener noreferrer" class="link">Read exactly what is collected</a>
        </p>
      </div>
    </Panel>
  );
}

function CommandLine({ command }: { command: string }) {
  return (
    <div class="flex items-center gap-2 rounded-[8px] border border-line bg-panel-2 pl-3 pr-1 py-1">
      <code class="flex-1 min-w-0 break-all text-[12.5px] text-fg">{command}</code>
      <CopyButton text={command} />
    </div>
  );
}

/* ------------------------------------------------------------------ Help */

function HelpPanel() {
  const links: { icon: IconName; label: string; body: string; href: string }[] = [
    { icon: 'book', label: 'Documentation', body: 'Guides for every database, roles, the REST API and MCP.', href: DOCS_URL },
    { icon: 'github', label: 'Report a bug or request a feature', body: 'Open an issue on GitHub. Include your version.', href: `${GITHUB_URL}/issues` },
    { icon: 'refresh', label: 'Release notes', body: 'What changed in each version, and how to upgrade.', href: `${GITHUB_URL}/releases` },
  ];
  return (
    <Panel title="Help">
      <ul class="divide-y divide-line">
        {links.map((l) => (
          <li key={l.href}>
            <a
              href={l.href}
              target="_blank"
              rel="noopener noreferrer"
              class="flex items-start gap-3 px-5 py-3 hover:bg-panel-2 transition-colors group"
            >
              <Icon name={l.icon} size={18} class="mt-0.5 text-fg-muted group-hover:text-fg shrink-0" />
              <span class="min-w-0 flex-1">
                <span class="block text-sm font-medium text-fg">{l.label}</span>
                <span class="block text-xs text-fg-muted mt-0.5">{l.body}</span>
              </span>
              <Icon name="external" size={14} class="mt-1 text-fg-faint shrink-0" />
            </a>
          </li>
        ))}
      </ul>
    </Panel>
  );
}
