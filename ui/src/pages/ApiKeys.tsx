import { useEffect, useMemo, useState } from 'preact/hooks';
import { CodeBlock, CodeSample } from '../components/CodeBlock';
import { ServiceRecord } from '../components/ConnectionForm';
import { Icon } from '../components/Icon';
import { Modal, confirm, toast } from '../components/Overlay';
import {
  Button,
  ButtonLink,
  EmptyState,
  Field,
  Input,
  Notice,
  PageHeader,
  Panel,
  Select,
  Skeleton,
  Tag,
  copyText,
  useId,
} from '../components/ui';
import { apiFetch, errorMessage } from '../hooks/useApi';
import { formatDate, plural, serverOrigin, shellQuote, timeAgo } from '../lib/format';

/* ------------------------------------------------------------------ Model */

interface ApiKey {
  id: number;
  label: string;
  key_prefix: string;
  role_id: number;
  role_name?: string;
  is_active: boolean;
  created_at: string;
  last_used?: string | null;
  expires_at?: string | null;
}

interface RoleAccess {
  service_name: string;
  component: string;
  verb_mask: number;
}

interface Role {
  id: number;
  name: string;
  is_active: boolean;
  access: RoleAccess[] | null;
}

/** Response of POST /api/v1/system/api-key. The plaintext key is only here. */
interface CreatedKey {
  id: number;
  api_key: string;
  key_prefix: string;
  label: string;
  role_id: number;
  expires_at?: string | null;
}

const EXPIRY_OPTIONS = [
  { value: '0', label: 'Never' },
  { value: '7', label: 'In 7 days' },
  { value: '30', label: 'In 30 days' },
  { value: '90', label: 'In 90 days' },
  { value: '365', label: 'In 1 year' },
];

const isExpired = (k: ApiKey) => !!k.expires_at && new Date(k.expires_at).getTime() < Date.now();

/* ---------------------------------------------------- Rule matching (TS)
   Mirrors internal/rbac/rbac.go so the sample request we print is one the
   new key is actually allowed to make. */

const GET = 1;

function matchService(pattern: string, service: string): boolean {
  const p = pattern.trim();
  if (p === '' || p === '*') return true;
  if (p.endsWith('*')) return service.startsWith(p.slice(0, -1));
  return p === service;
}

function matchComponent(pattern: string, component: string): boolean {
  const p = pattern.trim().replace(/^\/+|\/+$/g, '');
  const c = component;
  if (p === '' || p === '*' || p === c) return true;
  if (p.endsWith('*')) {
    const prefix = p.slice(0, -1);
    if (c.startsWith(prefix)) return true;
    return prefix.endsWith('/') && prefix.slice(0, -1) === c;
  }
  if (!p.includes('/')) {
    const i = c.indexOf('/');
    return i >= 0 && c.slice(i + 1) === p;
  }
  return false;
}

interface Sample {
  path: string;
  what: string;
  allowed: boolean;
  placeholder: boolean;
}

/** Pick a first GET request the role permits: list tables, or read one table. */
function sampleRequest(role: Role | undefined, services: ServiceRecord[]): Sample {
  const live = services.filter((s) => s.is_active);
  const pool = live.length ? live : services;
  for (const rule of role?.access || []) {
    if (!(rule.verb_mask & GET)) continue;
    for (const svc of pool) {
      if (!matchService(rule.service_name, svc.name)) continue;
      const base = `/api/v1/${svc.name}`;
      if (matchComponent(rule.component, '_table')) {
        return { path: `${base}/_table`, what: `Lists the tables in ${svc.name}.`, allowed: true, placeholder: false };
      }
      const comp = rule.component.trim().replace(/^\/+|\/+$/g, '');
      const table = !comp.includes('*') && (comp.startsWith('_table/') ? comp.slice(7) : !comp.includes('/') ? comp : '');
      if (table) {
        return {
          path: `${base}/_table/${encodeURIComponent(table)}?limit=5`,
          what: `Reads 5 rows from ${table} in ${svc.name}.`,
          allowed: true,
          placeholder: false,
        };
      }
    }
  }
  const first = pool[0];
  if (!first) {
    return { path: '/api/v1/{database}/_table', what: 'Replace {database} with the API name of a database.', allowed: true, placeholder: true };
  }
  return {
    path: `/api/v1/${first.name}/_table`,
    what: `Lists the tables in ${first.name}. This key's role does not allow it yet, so expect a 403 until you add a read rule.`,
    allowed: false,
    placeholder: false,
  };
}

function snippets(key: string, sample: Sample): CodeSample[] {
  const origin = serverOrigin();
  const url = `${origin}${sample.path}`;
  return [
    {
      label: 'curl',
      note: sample.what,
      code: `curl ${shellQuote(url)} \\\n  -H "X-API-Key: ${key}"`,
    },
    {
      label: 'JavaScript',
      note: sample.what,
      code: `const res = await fetch(${JSON.stringify(url)}, {\n  headers: { "X-API-Key": ${JSON.stringify(key)} },\n});\nconsole.log(await res.json());`,
    },
    {
      label: 'Claude Code',
      note: 'Gives Claude Code the MCP tools for every database this key can use.',
      code: `claude mcp add --transport http faucet ${origin}/mcp --header "X-API-Key: ${key}"`,
    },
  ];
}

/* ------------------------------------------------------------------ Page */

export function ApiKeys() {
  const [keys, setKeys] = useState<ApiKey[] | null>(null);
  const [roles, setRoles] = useState<Role[] | null>(null);
  const [services, setServices] = useState<ServiceRecord[]>([]);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [modal, setModal] = useState<{ open: boolean; roleId?: number }>({ open: false });
  const [showRevoked, setShowRevoked] = useState(false);
  const [flash, setFlash] = useState<number | null>(null);

  useEffect(() => {
    loadKeys();
    loadRoles();
    apiFetch('/api/v1/system/service')
      .then((res) => setServices(res.resource || []))
      .catch(() => setServices([]));

    // Deep link from the roles page: /api-keys?new=1&role=3 opens the form.
    const q = new URLSearchParams(window.location.search);
    if (q.get('new')) {
      const r = Number(q.get('role'));
      setModal({ open: true, roleId: Number.isFinite(r) && r > 0 ? r : undefined });
      window.history.replaceState(null, '', '/api-keys');
    }

    // Keep "last used" current while the page is open.
    const id = window.setInterval(loadKeys, 30_000);
    return () => window.clearInterval(id);
  }, []);

  async function loadKeys() {
    try {
      const res = await apiFetch('/api/v1/system/api-key');
      setKeys(res.resource || []);
      setLoadError(null);
    } catch (err) {
      setLoadError(errorMessage(err));
      setKeys((k) => k || []);
    }
  }

  async function loadRoles() {
    try {
      const res = await apiFetch('/api/v1/system/role');
      setRoles(res.resource || []);
    } catch {
      setRoles([]);
    }
  }

  const roleById = useMemo(() => new Map((roles || []).map((r) => [r.id, r])), [roles]);
  const active = (keys || []).filter((k) => k.is_active);
  const revoked = (keys || []).filter((k) => !k.is_active);
  const visible = showRevoked ? [...active, ...revoked] : active;

  async function revoke(k: ApiKey) {
    const ok = await confirm({
      title: `Revoke ${k.label || k.key_prefix}?`,
      body: k.last_used
        ? `Anything using this key stops working right away. It was last used ${timeAgo(k.last_used).toLowerCase()}. This cannot be undone.`
        : 'Anything using this key stops working right away. This cannot be undone.',
      confirmLabel: 'Revoke key',
      danger: true,
    });
    if (!ok) return;
    try {
      await apiFetch(`/api/v1/system/api-key/${k.id}`, { method: 'DELETE' });
      toast(`Key revoked: ${k.label || k.key_prefix}`);
      loadKeys();
    } catch (err) {
      toast(errorMessage(err, 'Could not revoke the key'), 'bad');
    }
  }

  const createButton = (
    <Button variant="primary" icon="plus" onClick={() => setModal({ open: true })}>
      Create key
    </Button>
  );

  return (
    <div>
      <PageHeader
        title="API keys"
        description={
          <>
            Keys authenticate apps and AI agents. Send one in the <span class="code-chip">X-API-Key</span> header. Faucet stores only a hash, so a key is shown once.
          </>
        }
        actions={keys && keys.length > 0 ? createButton : undefined}
      />

      {loadError && <p class="mb-4 text-sm text-bad">Could not load keys: {loadError}. Reload the page to try again.</p>}

      {keys === null ? (
        <Panel bodyClass="divide-y divide-line">
          {[0, 1].map((i) => (
            <div key={i} class="flex items-center gap-4 px-5 py-4">
              <Skeleton class="w-8 h-8 rounded-[8px]" />
              <div class="flex-1 flex flex-col gap-2">
                <Skeleton class="h-4 w-48" />
                <Skeleton class="h-3 w-72" />
              </div>
            </div>
          ))}
        </Panel>
      ) : keys.length === 0 ? (
        <Panel>
          <EmptyState icon="key" title="Create your first API key" action={createButton}>
            A key carries a role, so it can only reach the databases and tables that role allows. You'll get ready-to-run curl and Claude Code commands right after.
          </EmptyState>
        </Panel>
      ) : (
        <Panel bodyClass="divide-y divide-line">
          {visible.length === 0 && (
            <div class="px-5 py-6 text-sm text-fg-muted">
              Every key has been revoked.{' '}
              <button type="button" class="link" onClick={() => setModal({ open: true })}>
                Create a new key
              </button>
            </div>
          )}
          {visible.map((k) => {
            const expired = isExpired(k);
            const roleName = k.role_name || roleById.get(k.role_id)?.name;
            const role = roleById.get(k.role_id);
            return (
              <div key={k.id} class={`px-5 py-4 ${flash === k.id ? 'anim-flash' : ''} ${k.is_active ? '' : 'opacity-70'}`}>
                <div class="flex flex-wrap items-start gap-x-4 gap-y-3">
                  <span class="inline-flex items-center justify-center w-8 h-8 rounded-[8px] bg-panel-2 border border-line text-fg-muted shrink-0" aria-hidden="true">
                    <Icon name="key" size={16} />
                  </span>
                  <div class="min-w-0 flex-1 basis-64">
                    <div class="flex flex-wrap items-center gap-x-2.5 gap-y-1">
                      <h2 class="text-base font-semibold text-fg break-all">{k.label || 'Unnamed key'}</h2>
                      <code class="text-[12.5px] text-fg-muted">{k.key_prefix}…</code>
                      {!k.is_active ? <Tag>Revoked</Tag> : expired ? <Tag tone="bad">Expired</Tag> : null}
                      {k.is_active && role && !role.is_active && <Tag tone="warn">Role inactive</Tag>}
                    </div>
                    <dl class="mt-1.5 flex flex-wrap gap-x-6 gap-y-1 text-sm">
                      <div class="flex gap-1.5">
                        <dt class="text-fg-faint">Role</dt>
                        <dd class="text-fg">
                          {roleName ? (
                            <a class="link" href="/roles">{roleName}</a>
                          ) : (
                            <span class="text-bad">Deleted role</span>
                          )}
                        </dd>
                      </div>
                      <div class="flex gap-1.5">
                        <dt class="text-fg-faint">Last used</dt>
                        <dd class="text-fg-muted" title={k.last_used ? new Date(k.last_used).toLocaleString() : undefined}>
                          {timeAgo(k.last_used)}
                        </dd>
                      </div>
                      <div class="flex gap-1.5">
                        <dt class="text-fg-faint">{expired ? 'Expired' : 'Expires'}</dt>
                        <dd class={expired ? 'text-bad' : 'text-fg-muted'}>{k.expires_at ? formatDate(k.expires_at) : 'Never'}</dd>
                      </div>
                      <div class="flex gap-1.5">
                        <dt class="text-fg-faint">Created</dt>
                        <dd class="text-fg-muted">{formatDate(k.created_at)}</dd>
                      </div>
                    </dl>
                  </div>
                  {k.is_active && (
                    <Button size="sm" variant="danger" onClick={() => revoke(k)}>
                      Revoke
                    </Button>
                  )}
                </div>
              </div>
            );
          })}
          {revoked.length > 0 && (
            <div class="px-5 py-2.5">
              <Button size="sm" variant="ghost" onClick={() => setShowRevoked(!showRevoked)}>
                {showRevoked ? 'Hide revoked keys' : `Show ${plural(revoked.length, 'revoked key')}`}
              </Button>
            </div>
          )}
        </Panel>
      )}

      <CreateKeyModal
        open={modal.open}
        initialRoleId={modal.roleId}
        roles={roles}
        services={services}
        onClose={() => setModal({ open: false })}
        onCreated={(k) => {
          setFlash(k.id);
          loadKeys();
        }}
      />
    </div>
  );
}

/* ------------------------------------------------------------ Create key */

function CreateKeyModal({ open, initialRoleId, roles, services, onClose, onCreated }: {
  open: boolean;
  initialRoleId?: number;
  roles: Role[] | null;
  services: ServiceRecord[];
  onClose: () => void;
  onCreated: (k: CreatedKey) => void;
}) {
  const [label, setLabel] = useState('');
  const [roleId, setRoleId] = useState('');
  const [expiry, setExpiry] = useState('0');
  const [labelError, setLabelError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [created, setCreated] = useState<CreatedKey | null>(null);
  const [copied, setCopied] = useState(false);
  const labelId = useId('key-label');
  const roleSel = useId('key-role');
  const expId = useId('key-exp');
  const keyId = useId('key-value');

  const list = roles || [];
  const selectedRole = list.find((r) => String(r.id) === roleId);

  useEffect(() => {
    if (!open) return;
    setLabel('');
    setExpiry('0');
    setLabelError(null);
    setSaveError(null);
    setCreated(null);
    setCopied(false);
  }, [open]);

  // Pick the requested role, or the first active one, once roles are known.
  useEffect(() => {
    if (!open || !roles) return;
    const wanted = initialRoleId && roles.find((r) => r.id === initialRoleId);
    const pick = wanted || roles.find((r) => r.is_active) || roles[0];
    setRoleId(pick ? String(pick.id) : '');
  }, [open, roles, initialRoleId]);

  async function create() {
    if (!label.trim()) {
      setLabelError('Name the key after what will use it, for example "billing-service" or "Claude Desktop".');
      return;
    }
    setLabelError(null);
    setSaving(true);
    setSaveError(null);
    try {
      const body: { label: string; role_id: number; expires_at?: string } = { label: label.trim(), role_id: Number(roleId) };
      const days = Number(expiry);
      if (days > 0) body.expires_at = new Date(Date.now() + days * 86_400_000).toISOString();
      const res = await apiFetch<CreatedKey>('/api/v1/system/api-key', { method: 'POST', body });
      setCreated(res);
      toast(`Key created: ${res.label}`);
      onCreated(res);
    } catch (err) {
      setSaveError(errorMessage(err, 'Could not create the key'));
    } finally {
      setSaving(false);
    }
  }

  async function close() {
    if (created && !copied) {
      const ok = await confirm({
        title: 'Close without copying the key?',
        body: "Faucet can't show this key again. If you close now, you'll need to revoke it and create another.",
        confirmLabel: 'Close anyway',
        danger: true,
      });
      if (!ok) return;
    }
    onClose();
  }

  /* After creation: show the key once, with ready-to-run snippets. */
  if (created) {
    const role = list.find((r) => r.id === created.role_id);
    const sample = sampleRequest(role, services);
    return (
      <Modal
        open={open}
        onClose={close}
        title="Copy your new key"
        width="max-w-2xl"
        footer={
          <Button variant="primary" onClick={close}>
            Done
          </Button>
        }
      >
        <div class="flex flex-col gap-4">
          <Notice tone="warn" title="This is the only time you'll see this key">
            Faucet keeps only a hash. Store the key in your secrets manager or .env file now.
          </Notice>

          <div class="flex flex-col gap-1.5">
            <label for={keyId} class="text-sm font-medium text-fg">
              API key
            </label>
            <div class="flex flex-col sm:flex-row gap-2">
              <input
                id={keyId}
                readOnly
                value={created.api_key}
                onFocus={(e) => (e.target as HTMLInputElement).select()}
                class="flex-1 min-w-0 h-10 px-3 rounded-[var(--radius-control)] bg-panel-2 border border-line-strong font-mono text-[13px] text-fg focus:outline-none focus:border-brand focus:ring-2 focus:ring-brand/25"
                spellcheck={false}
              />
              <Button
                variant="primary"
                icon={copied ? 'check' : 'copy'}
                class="h-10"
                onClick={async () => {
                  if (await copyText(created.api_key)) {
                    setCopied(true);
                    toast('Key copied');
                  }
                }}
              >
                {copied ? 'Copied' : 'Copy key'}
              </Button>
            </div>
            <p class="text-xs text-fg-muted">
              Role {role ? role.name : 'unknown'}
              {created.expires_at ? `, expires ${formatDate(created.expires_at)}` : ', never expires'}.
            </p>
          </div>

          <div class="flex flex-col gap-2">
            <h3 class="text-base font-semibold text-fg">Try it</h3>
            {/* Copying a snippet also copies the key, so count it. */}
            <div
              onClickCapture={(e) => {
                const btn = (e.target as HTMLElement).closest('button');
                if (btn && btn.getAttribute('role') !== 'tab') setCopied(true);
              }}
            >
              <CodeBlock samples={snippets(created.api_key, sample)} />
            </div>
            {!sample.allowed && <Notice tone="info">Edit the role on the <a class="link" href="/roles">Roles</a> page to give this key read access.</Notice>}
            <p class="text-sm text-fg-muted">
              Using Claude Desktop, Cursor or another client? The <a class="link" href="/mcp">MCP page</a> has a config for each.
            </p>
          </div>
        </div>
      </Modal>
    );
  }

  /* No roles yet: a key can't be created without one. */
  if (roles && roles.length === 0) {
    return (
      <Modal
        open={open}
        onClose={onClose}
        title="Create a role first"
        footer={
          <>
            <Button variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <ButtonLink variant="primary" href="/roles">
              Go to roles
            </ButtonLink>
          </>
        }
      >
        <p class="text-base text-fg-muted">
          Every key carries a role that decides which databases and tables it can use. Create one, for example "Read-only on everything", then come back here.
        </p>
      </Modal>
    );
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Create API key"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={saving} disabled={!roleId} onClick={create}>
            Create key
          </Button>
        </>
      }
    >
      <form
        class="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (roleId) create();
        }}
      >
        {saveError && <Notice tone="bad" title="The key was not created">{saveError}</Notice>}
        <Field label="Name" htmlFor={labelId} error={labelError} hint="Who or what will use it. You'll see this in the list.">
          <Input id={labelId} value={label} onValue={setLabel} placeholder="billing-service" invalid={!!labelError} autoComplete="off" />
        </Field>
        <Field
          label="Role"
          htmlFor={roleSel}
          hint={
            selectedRole && !selectedRole.is_active ? (
              <span class="text-warn">This role is inactive, so the key will be refused until you turn the role on.</span>
            ) : (
              <>
                Decides what the key can reach. <a class="link" href="/roles">Manage roles</a>
              </>
            )
          }
        >
          {roles === null ? (
            <Skeleton class="h-9 w-full" />
          ) : (
            <Select id={roleSel} value={roleId} onValue={setRoleId} options={list.map((r) => ({ value: String(r.id), label: r.is_active ? r.name : `${r.name} (inactive)` }))} />
          )}
        </Field>
        <Field label="Expires" htmlFor={expId}>
          <Select id={expId} value={expiry} onValue={setExpiry} options={EXPIRY_OPTIONS} />
        </Field>
        <button type="submit" class="hidden" tabIndex={-1} aria-hidden="true" />
      </form>
    </Modal>
  );
}
