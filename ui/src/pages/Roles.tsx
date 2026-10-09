import { ComponentChildren } from 'preact';
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { ServiceRecord } from '../components/ConnectionForm';
import { Icon } from '../components/Icon';
import { Drawer, confirm, toast } from '../components/Overlay';
import {
  Button,
  EmptyState,
  Field,
  IconButton,
  Input,
  Notice,
  PageHeader,
  Panel,
  Segmented,
  Select,
  Skeleton,
  StatusDot,
  Switch,
  useId,
} from '../components/ui';
import { apiFetch, errorMessage } from '../hooks/useApi';
import { plural } from '../lib/format';

/* ------------------------------------------------------------------ Model */

// Verb bits, matching internal/model/role.go.
const GET = 1;
const POST = 2;
const PUT = 4;
const PATCH = 8;
const DELETE = 16;
const ALL = GET | POST | PUT | PATCH | DELETE;
const READ_WRITE = GET | POST | PUT | PATCH;

const VERBS: { bit: number; word: string; method: string }[] = [
  { bit: GET, word: 'Read', method: 'GET' },
  { bit: POST, word: 'Create', method: 'POST' },
  { bit: PUT, word: 'Replace', method: 'PUT' },
  { bit: PATCH, word: 'Update', method: 'PATCH' },
  { bit: DELETE, word: 'Delete', method: 'DELETE' },
];

interface Filter {
  name: string;
  operator: string;
  value: string;
}

interface RoleAccess {
  id?: number;
  service_name: string;
  component: string;
  verb_mask: number;
  requestor_mask?: number;
  filters?: Filter[];
  filter_op?: string;
}

interface Role {
  id: number;
  name: string;
  description: string;
  is_active: boolean;
  access: RoleAccess[] | null;
  created_at?: string;
  updated_at?: string;
}

interface KeyRecord {
  id: number;
  role_id: number;
  is_active: boolean;
}

type Level = 'read' | 'write' | 'full' | 'custom';

function levelForMask(mask: number): Level {
  if (mask === GET) return 'read';
  if (mask === READ_WRITE) return 'write';
  if (mask === ALL) return 'full';
  return 'custom';
}

const LEVEL_MASK: Record<Exclude<Level, 'custom'>, number> = { read: GET, write: READ_WRITE, full: ALL };

/** What a verb mask allows, in words: "Read", "Read and write", "Read, create and delete". */
function levelWords(mask: number): string {
  if (mask === GET) return 'Read';
  if (mask === READ_WRITE) return 'Read and write';
  if (mask === ALL) return 'Full access';
  const words = VERBS.filter((v) => mask & v.bit).map((v) => v.word);
  if (words.length === 0) return 'No access';
  const lower = words.map((w, i) => (i === 0 ? w : w.toLowerCase()));
  return lower.length === 1 ? lower[0] : `${lower.slice(0, -1).join(', ')} and ${lower[lower.length - 1]}`;
}

const isAll = (p: string) => p.trim() === '' || p.trim() === '*';

function Mono({ children }: { children: ComponentChildren }) {
  return <span class="font-mono text-[12.5px] text-fg">{children}</span>;
}

function databaseWords(service: string): ComponentChildren {
  if (isAll(service)) return 'all databases';
  if (service.endsWith('*')) return <>databases starting with <Mono>{service.slice(0, -1)}</Mono></>;
  return <Mono>{service}</Mono>;
}

/** One rule as a sentence: "Read on shop (all tables)", "Full access on orders in shop". */
function ruleSentence(r: RoleAccess): ComponentChildren {
  const level = levelWords(r.verb_mask);
  const comp = r.component.trim().replace(/^\/+|\/+$/g, '');
  if (isAll(comp)) {
    return <>{level} on {databaseWords(r.service_name)} (all tables)</>;
  }
  const bare = !comp.includes('/') && !comp.includes('*');
  return (
    <>
      {level} on {bare ? <>table </> : null}
      <Mono>{comp}</Mono> in {databaseWords(r.service_name)}
    </>
  );
}

/** Short summary for a role row. */
function roleSummary(access: RoleAccess[]): ComponentChildren[] {
  if (access.length === 0) return [];
  const sameMask = access.every((r) => r.verb_mask === access[0].verb_mask);
  const allTables = access.every((r) => isAll(r.component));
  const exactDbs = access.every((r) => !isAll(r.service_name) && !r.service_name.endsWith('*'));
  if (access.length > 1 && sameMask && allTables && exactDbs) {
    const names = access.map((r) => r.service_name);
    const level = levelWords(access[0].verb_mask);
    if (names.length === 2) {
      return [<>{level} on <Mono>{names[0]}</Mono> and <Mono>{names[1]}</Mono> (all tables)</>];
    }
    return [<>{level} on {names.length} databases (all tables)</>];
  }
  const lines = access.slice(0, 2).map(ruleSentence);
  if (access.length > 2) lines.push(<>and {plural(access.length - 2, 'more rule')}</>);
  return lines;
}

/* ----------------------------------------------------------- Form state */

const PATTERN = '__pattern__';

interface RuleForm {
  key: number;
  service: string;
  /** Show a free-text name pattern instead of the database picker. */
  servicePattern: boolean;
  /** "" means all tables (stored as "*"). */
  component: string;
  mask: number;
  level: Level;
  // Carried through untouched so editing a role never drops stored data.
  requestor_mask: number;
  filters: Filter[];
  filter_op: string;
}

interface RoleForm {
  name: string;
  description: string;
  is_active: boolean;
  rules: RuleForm[];
}

let ruleSeq = 0;

function newRule(service = '*'): RuleForm {
  return {
    key: ++ruleSeq,
    service,
    servicePattern: false,
    component: '',
    mask: GET,
    level: 'read',
    requestor_mask: 1,
    filters: [],
    filter_op: 'AND',
  };
}

function ruleFromAccess(a: RoleAccess, serviceNames: string[]): RuleForm {
  const service = a.service_name || '*';
  return {
    key: ++ruleSeq,
    service,
    servicePattern: !isAll(service) && service.endsWith('*') && !serviceNames.includes(service),
    component: isAll(a.component) ? '' : a.component,
    mask: a.verb_mask,
    level: levelForMask(a.verb_mask),
    requestor_mask: a.requestor_mask ?? 1,
    filters: a.filters || [],
    filter_op: a.filter_op || 'AND',
  };
}

function ruleToAccess(r: RuleForm): RoleAccess {
  return {
    service_name: r.service.trim() || '*',
    component: r.component.trim() || '*',
    verb_mask: r.mask,
    requestor_mask: r.requestor_mask,
    filters: r.filters,
    filter_op: r.filter_op,
  };
}

const TEMPLATES: { name: string; description: string; mask: number; label: string; hint: string }[] = [
  {
    label: 'Read-only on everything',
    hint: 'Read every table in every database. Good for dashboards and AI agents.',
    name: 'Read-only',
    description: 'Read every table in every database',
    mask: GET,
  },
  {
    label: 'Full access to everything',
    hint: 'Read, write and delete in every database. Keep keys with this role on servers you trust.',
    name: 'Full access',
    description: 'Read, write and delete in every database',
    mask: ALL,
  },
];

/* ------------------------------------------------------------------ Page */

export function Roles() {
  const [roles, setRoles] = useState<Role[] | null>(null);
  const [keys, setKeys] = useState<KeyRecord[]>([]);
  const [services, setServices] = useState<ServiceRecord[]>([]);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [editor, setEditor] = useState<{ open: boolean; role?: Role | null }>({ open: false });
  const [flash, setFlash] = useState<number | null>(null);
  const [creatingTemplate, setCreatingTemplate] = useState<string | null>(null);

  useEffect(() => {
    load();
    apiFetch('/api/v1/system/service')
      .then((res) => setServices(res.resource || []))
      .catch(() => setServices([]));
  }, []);

  async function load() {
    const [r, k] = await Promise.allSettled([apiFetch('/api/v1/system/role'), apiFetch('/api/v1/system/api-key')]);
    if (r.status === 'fulfilled') {
      setRoles(r.value.resource || []);
      setLoadError(null);
    } else {
      setRoles((prev) => prev || []);
      setLoadError(errorMessage(r.reason));
    }
    if (k.status === 'fulfilled') setKeys(k.value.resource || []);
  }

  const keyCounts = useMemo(() => {
    const m = new Map<number, { active: number; total: number }>();
    for (const k of keys) {
      const c = m.get(k.role_id) || { active: 0, total: 0 };
      c.total++;
      if (k.is_active) c.active++;
      m.set(k.role_id, c);
    }
    return m;
  }, [keys]);

  async function createTemplate(t: (typeof TEMPLATES)[number]) {
    setCreatingTemplate(t.name);
    try {
      const role = await apiFetch<Role>('/api/v1/system/role', {
        method: 'POST',
        body: {
          name: uniqueName(t.name, roles || []),
          description: t.description,
          access: [{ service_name: '*', component: '*', verb_mask: t.mask, requestor_mask: 1 }],
        },
      });
      toast(`Role created: ${role.name}`);
      setFlash(role.id);
      load();
    } catch (err) {
      toast(errorMessage(err, 'Could not create the role'), 'bad');
    } finally {
      setCreatingTemplate(null);
    }
  }

  async function deactivate(role: Role) {
    await apiFetch(`/api/v1/system/role/${role.id}`, { method: 'PUT', body: { is_active: false } });
    toast(`Role deactivated: ${role.name}`);
    load();
  }

  async function remove(role: Role) {
    const counts = keyCounts.get(role.id);
    const active = counts?.active || 0;
    const ok = await confirm({
      title: `Delete ${role.name}?`,
      body:
        active > 0 ? (
          <>
            {plural(active, 'active key')} {active === 1 ? 'uses' : 'use'} this role. {active === 1 ? 'It stops' : 'They stop'} working right away.
          </>
        ) : (
          'No active keys use this role. This cannot be undone.'
        ),
      confirmLabel: 'Delete role',
      danger: true,
    });
    if (!ok) return;
    try {
      await apiFetch(`/api/v1/system/role/${role.id}`, { method: 'DELETE' });
      toast(`Role deleted: ${role.name}`);
      load();
    } catch (err) {
      const msg = errorMessage(err);
      // Keys (revoked ones too) keep a reference to their role, so the
      // config store refuses the delete. Offer the equivalent instead.
      if (/foreign key/i.test(msg) || (counts && counts.total > 0)) {
        if (!role.is_active) {
          toast(`${role.name} still has keys on record, so it can't be deleted. It is already inactive, so those keys are refused.`, 'bad');
          return;
        }
        const off = await confirm({
          title: `${role.name} still has keys`,
          body: (
            <>
              Faucet keeps revoked keys on record and they still point to this role, so it can't be deleted. Deactivate it instead: every key with this role is refused.
            </>
          ),
          confirmLabel: 'Deactivate role',
          danger: true,
        });
        if (off) {
          try {
            await deactivate(role);
          } catch (e2) {
            toast(errorMessage(e2, 'Could not deactivate the role'), 'bad');
          }
        }
        return;
      }
      toast(msg || 'Could not delete the role', 'bad');
    }
  }

  function onSaved(role: Role, created: boolean) {
    setEditor({ open: false });
    setFlash(role.id);
    toast(created ? `Role created: ${role.name}` : `Changes saved to ${role.name}`);
    load();
  }

  const createButton = (
    <Button variant="primary" icon="plus" onClick={() => setEditor({ open: true })}>
      Create role
    </Button>
  );

  return (
    <div>
      <PageHeader
        title="Roles"
        description="A role decides which databases and tables an API key can use, and what it can do there."
        actions={roles && roles.length > 0 ? createButton : undefined}
      />

      {loadError && <p class="mb-4 text-sm text-bad">Could not load roles: {loadError}. Reload the page to try again.</p>}

      {roles === null ? (
        <Panel bodyClass="divide-y divide-line">
          {[0, 1].map((i) => (
            <div key={i} class="flex items-center gap-4 px-5 py-4">
              <Skeleton class="w-8 h-8 rounded-[8px]" />
              <div class="flex-1 flex flex-col gap-2">
                <Skeleton class="h-4 w-40" />
                <Skeleton class="h-3 w-72" />
              </div>
            </div>
          ))}
        </Panel>
      ) : roles.length === 0 ? (
        <Panel>
          <EmptyState icon="shield" title="Create your first role">
            Every API key carries a role. Start with one of these, or build your own rules per database and table.
          </EmptyState>
          <div class="px-6 pb-8 sm:px-10 -mt-2 flex flex-col gap-2 max-w-[640px]">
            {TEMPLATES.map((t) => (
              <div key={t.name} class="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3 rounded-[8px] border border-line">
                <div class="flex-1 min-w-[220px]">
                  <p class="text-base font-medium text-fg">{t.label}</p>
                  <p class="text-sm text-fg-muted">{t.hint}</p>
                </div>
                <Button size="sm" variant="secondary" aria-label={`Create role: ${t.label}`} loading={creatingTemplate === t.name} disabled={!!creatingTemplate} onClick={() => createTemplate(t)}>
                  Create role
                </Button>
              </div>
            ))}
            <div class="mt-2">
              <Button variant="primary" icon="plus" onClick={() => setEditor({ open: true })}>
                Create custom role
              </Button>
            </div>
          </div>
        </Panel>
      ) : (
        <Panel bodyClass="divide-y divide-line">
          {roles.map((role) => {
            const access = role.access || [];
            const counts = keyCounts.get(role.id);
            const active = counts?.active || 0;
            const summary = roleSummary(access);
            return (
              <div key={role.id} class={`px-5 py-4 ${flash === role.id ? 'anim-flash' : ''}`}>
                <div class="flex flex-wrap items-start gap-x-4 gap-y-3">
                  <span class="inline-flex items-center justify-center w-8 h-8 rounded-[8px] bg-panel-2 border border-line text-fg-muted shrink-0" aria-hidden="true">
                    <Icon name="shield" size={16} />
                  </span>
                  <div class="min-w-0 flex-1 basis-60">
                    <div class="flex flex-wrap items-center gap-x-2.5 gap-y-1">
                      <h2 class="text-base font-semibold text-fg break-all">{role.name}</h2>
                      {role.is_active ? <StatusDot status="ok" label="Active" /> : <StatusDot status="idle" label="Inactive, keys are refused" />}
                    </div>
                    {role.description && <p class="text-sm text-fg-muted mt-0.5">{role.description}</p>}
                    <div class="mt-2 flex flex-col gap-0.5 text-sm text-fg-muted">
                      {summary.length === 0 ? (
                        <p class="text-warn">
                          <Icon name="alert" size={14} class="inline -mt-0.5 mr-1.5" />
                          No rules yet, so keys with this role can't do anything.
                        </p>
                      ) : (
                        summary.map((line, i) => <p key={i}>{line}</p>)
                      )}
                    </div>
                    <p class="mt-2 text-sm text-fg-muted">
                      {active === 0 ? (
                        <>
                          No active keys.{' '}
                          <a class="link" href={`/api-keys?new=1&role=${role.id}`}>
                            Create a key with this role
                          </a>
                        </>
                      ) : (
                        <a class="link" href="/api-keys">
                          Used by {plural(active, 'active key')}
                        </a>
                      )}
                    </p>
                  </div>
                  <div class="flex items-center gap-1.5">
                    <Button size="sm" variant="ghost" icon="pencil" onClick={() => setEditor({ open: true, role })}>
                      Edit
                    </Button>
                    <IconButton icon="trash" label={`Delete ${role.name}`} onClick={() => remove(role)} class="hover:!text-bad" />
                  </div>
                </div>
              </div>
            );
          })}
        </Panel>
      )}

      <RoleEditor
        open={editor.open}
        role={editor.role || null}
        roles={roles || []}
        services={services}
        onClose={() => setEditor({ open: false })}
        onSaved={onSaved}
      />
    </div>
  );
}

function uniqueName(base: string, roles: Role[]): string {
  const taken = new Set(roles.map((r) => r.name.toLowerCase()));
  if (!taken.has(base.toLowerCase())) return base;
  for (let i = 2; ; i++) {
    const n = `${base} ${i}`;
    if (!taken.has(n.toLowerCase())) return n;
  }
}

/* ---------------------------------------------------------------- Editor */

function RoleEditor({ open, role, roles, services, onClose, onSaved }: {
  open: boolean;
  role: Role | null;
  roles: Role[];
  services: ServiceRecord[];
  onClose: () => void;
  onSaved: (role: Role, created: boolean) => void;
}) {
  const serviceNames = useMemo(() => services.map((s) => s.name), [services]);
  const [form, setForm] = useState<RoleForm>({ name: '', description: '', is_active: true, rules: [] });
  const [errors, setErrors] = useState<{ name?: string; rules?: string }>({});
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [tables, setTables] = useState<Record<string, string[] | 'loading' | 'error'>>({});
  const nameId = useId('role-name');
  const descId = useId('role-desc');
  const tableReq = useRef(new Set<string>());

  useEffect(() => {
    if (!open) return;
    setErrors({});
    setSaveError(null);
    if (role) {
      setForm({
        name: role.name,
        description: role.description || '',
        is_active: role.is_active,
        rules: (role.access || []).map((a) => ruleFromAccess(a, serviceNames)),
      });
    } else {
      setForm({ name: '', description: '', is_active: true, rules: [newRule(serviceNames.length === 1 ? serviceNames[0] : '*')] });
    }
  }, [open, role]);

  // Fetch table names for each concrete database used in a rule, once.
  useEffect(() => {
    if (!open) return;
    for (const r of form.rules) {
      const s = r.service.trim();
      if (r.servicePattern || isAll(s) || !serviceNames.includes(s) || tableReq.current.has(s)) continue;
      tableReq.current.add(s);
      setTables((t) => ({ ...t, [s]: 'loading' }));
      apiFetch(`/api/v1/${encodeURIComponent(s)}/_table`)
        .then((res) => setTables((t) => ({ ...t, [s]: (res.resource || []).map((x: { name: string }) => x.name) })))
        .catch(() => {
          tableReq.current.delete(s);
          setTables((t) => ({ ...t, [s]: 'error' }));
        });
    }
  }, [open, form.rules, serviceNames]);

  function patchRule(key: number, patch: Partial<RuleForm>) {
    setForm((f) => ({ ...f, rules: f.rules.map((r) => (r.key === key ? { ...r, ...patch } : r)) }));
  }

  function validate(): boolean {
    const e: { name?: string; rules?: string } = {};
    const name = form.name.trim();
    if (!name) e.name = 'Give the role a name, for example "Reporting" or "Support agent".';
    else if (roles.some((r) => r.id !== role?.id && r.name.toLowerCase() === name.toLowerCase())) e.name = `A role named ${name} already exists. Pick another name.`;
    if (form.rules.some((r) => r.mask === 0)) e.rules = 'Every rule needs at least one permission. Tick one or remove the rule.';
    else if (form.rules.some((r) => r.servicePattern && !r.service.trim())) e.rules = 'Enter a database name pattern, or pick a database.';
    setErrors(e);
    return !e.name && !e.rules;
  }

  async function save() {
    if (!validate()) return;
    setSaving(true);
    setSaveError(null);
    const access = form.rules.map(ruleToAccess);
    const name = form.name.trim();
    try {
      let saved: Role;
      if (role) {
        saved = await apiFetch<Role>(`/api/v1/system/role/${role.id}`, {
          method: 'PUT',
          body: { name, description: form.description.trim(), is_active: form.is_active, access },
        });
      } else {
        saved = await apiFetch<Role>('/api/v1/system/role', {
          method: 'POST',
          body: { name, description: form.description.trim(), access },
        });
        // The create endpoint always makes roles active.
        if (!form.is_active) {
          saved = await apiFetch<Role>(`/api/v1/system/role/${saved.id}`, { method: 'PUT', body: { is_active: false } });
        }
      }
      onSaved(saved, !role);
    } catch (err) {
      const msg = errorMessage(err, 'Could not save the role');
      setSaveError(/unique/i.test(msg) ? `A role named ${name} already exists. Pick another name.` : msg);
    } finally {
      setSaving(false);
    }
  }

  const clearedDescription = !!role && !!role.description && !form.description.trim();

  return (
    <Drawer
      open={open}
      onClose={onClose}
      title={role ? `Edit ${role.name}` : 'Create role'}
      description="Keys with this role can do only what these rules allow. Everything else is refused."
      width="max-w-[680px]"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={saving} onClick={save}>
            {role ? 'Save changes' : 'Create role'}
          </Button>
        </>
      }
    >
      <form
        class="flex flex-col gap-5"
        onSubmit={(e) => {
          e.preventDefault();
          save();
        }}
      >
        {saveError && <Notice tone="bad" title="The role was not saved">{saveError}</Notice>}

        <div class="grid gap-4 sm:grid-cols-2">
          <Field label="Name" htmlFor={nameId} error={errors.name}>
            <Input id={nameId} value={form.name} onValue={(v) => setForm({ ...form, name: v })} placeholder="Reporting" invalid={!!errors.name} autoComplete="off" />
          </Field>
          <Field label="Description" htmlFor={descId} optional hint={clearedDescription ? 'Faucet keeps the old description when this is left empty.' : undefined}>
            <Input id={descId} value={form.description} onValue={(v) => setForm({ ...form, description: v })} placeholder="Read-only access for dashboards" />
          </Field>
        </div>

        <Switch
          checked={form.is_active}
          onChange={(v) => setForm({ ...form, is_active: v })}
          label="Active"
          description="When off, every key with this role is refused until you turn it back on."
        />

        <section class="flex flex-col gap-3 pt-4 border-t border-line" aria-labelledby={`${nameId}-rules`}>
          <div class="flex items-start justify-between gap-4">
            <div>
              <h3 id={`${nameId}-rules`} class="text-base font-semibold text-fg">Access rules</h3>
              <p class="text-sm text-fg-muted mt-0.5">A request is allowed when any rule covers it. Names are case-sensitive.</p>
            </div>
            <Button size="sm" variant="secondary" icon="plus" onClick={() => setForm({ ...form, rules: [...form.rules, newRule()] })}>
              Add rule
            </Button>
          </div>

          {errors.rules && <p class="text-sm text-bad" role="alert">{errors.rules}</p>}

          {form.rules.length === 0 ? (
            <div class="px-4 py-5 rounded-[8px] border border-dashed border-line-strong text-sm text-fg-muted">
              No rules yet. Keys with this role will be refused everywhere.{' '}
              <button type="button" class="link" onClick={() => setForm({ ...form, rules: [newRule()] })}>
                Add a rule
              </button>
            </div>
          ) : (
            form.rules.map((r, i) => (
              <RuleEditor
                key={r.key}
                index={i}
                rule={r}
                services={services}
                tables={!r.servicePattern && !isAll(r.service) ? tables[r.service.trim()] : undefined}
                onChange={(patch) => patchRule(r.key, patch)}
                onRemove={() => setForm({ ...form, rules: form.rules.filter((x) => x.key !== r.key) })}
              />
            ))
          )}

          <PatternHelp />
        </section>

        <section class="pt-4 border-t border-line">
          <h3 class="text-base font-semibold text-fg">What this role allows</h3>
          <Preview form={form} />
        </section>

        {/* Lets Enter in a text field submit the form. */}
        <button type="submit" class="hidden" tabIndex={-1} aria-hidden="true" />
      </form>
    </Drawer>
  );
}

function RuleEditor({ index, rule, services, tables, onChange, onRemove }: {
  index: number;
  rule: RuleForm;
  services: ServiceRecord[];
  tables?: string[] | 'loading' | 'error';
  onChange: (patch: Partial<RuleForm>) => void;
  onRemove: () => void;
}) {
  const dbId = useId('rule-db');
  const patId = useId('rule-pat');
  const tblId = useId('rule-tbl');
  const listId = useId('rule-tables');
  const names = services.map((s) => s.name);
  const svc = rule.service.trim();

  const dbOptions = [
    { value: '*', label: 'All databases' },
    ...services.map((s) => ({ value: s.name, label: s.is_active ? s.name : `${s.name} (paused)` })),
  ];
  if (!rule.servicePattern && !isAll(svc) && !names.includes(svc)) {
    dbOptions.push({ value: svc, label: `${svc} (not connected)` });
  }
  dbOptions.push({ value: PATTERN, label: 'Name pattern…' });

  const tableList = Array.isArray(tables) ? tables : [];
  const tableHint =
    tables === 'loading'
      ? 'Loading tables…'
      : tables === 'error'
        ? 'Could not list tables for this database. You can still type a name.'
        : rule.component.trim() === ''
          ? 'Leave empty for all tables, or type a table name.'
          : rule.component.trim().includes('/') || rule.component.includes('*')
            ? 'Pattern. See "How matching works" below.'
            : `Covers the rows and schema of ${rule.component.trim()}.`;

  return (
    <fieldset class="rounded-[8px] border border-line bg-panel-2/40 p-4 flex flex-col gap-4 min-w-0">
      <legend class="sr-only">Rule {index + 1}</legend>
      <div class="grid gap-3 sm:grid-cols-2">
        <div class="flex flex-col gap-2 min-w-0">
          <Field label="Database" htmlFor={dbId}>
            <Select
              id={dbId}
              value={rule.servicePattern ? PATTERN : isAll(svc) ? '*' : svc}
              options={dbOptions}
              onValue={(v) =>
                v === PATTERN
                  ? onChange({ servicePattern: true, service: isAll(svc) ? '' : svc.endsWith('*') ? svc : `${svc}*` })
                  : onChange({ servicePattern: false, service: v, component: v === svc ? rule.component : '' })
              }
            />
          </Field>
          {rule.servicePattern && (
            <Field label="Database name pattern" htmlFor={patId} hint="End with * to match a prefix, e.g. prod_*">
              <Input id={patId} mono value={rule.service} onValue={(v) => onChange({ service: v })} placeholder="prod_*" autoComplete="off" spellcheck={false} />
            </Field>
          )}
        </div>
        <Field label="Tables" htmlFor={tblId} hint={tableHint}>
          <div class="flex gap-1.5">
            <Input
              id={tblId}
              mono
              list={tableList.length ? listId : undefined}
              value={rule.component}
              onValue={(v) => onChange({ component: v === '*' ? '' : v })}
              placeholder="All tables"
              autoComplete="off"
              spellcheck={false}
            />
            {rule.component.trim() !== '' && (
              <Button size="md" variant="ghost" onClick={() => onChange({ component: '' })} class="shrink-0">
                All tables
              </Button>
            )}
          </div>
          {tableList.length > 0 && (
            <datalist id={listId}>
              {tableList.map((t) => (
                <option key={t} value={t} />
              ))}
            </datalist>
          )}
        </Field>
      </div>

      <div class="flex flex-col gap-2">
        <span class="text-sm font-medium text-fg" id={`${dbId}-lvl`}>
          Access
        </span>
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="max-w-full overflow-x-auto">
            <Segmented<Level>
              label={`Access for rule ${index + 1}`}
              value={rule.level}
              onChange={(level) => onChange(level === 'custom' ? { level } : { level, mask: LEVEL_MASK[level] })}
              options={[
                { value: 'read', label: 'Read only' },
                { value: 'write', label: 'Read and write' },
                { value: 'full', label: 'Full' },
                { value: 'custom', label: 'Custom' },
              ]}
            />
          </div>
          <Button size="sm" variant="ghost" icon="trash" onClick={onRemove} aria-label={`Remove rule ${index + 1}`} class="hover:!text-bad">
            Remove
          </Button>
        </div>
        {rule.level === 'custom' ? (
          <div class="flex flex-wrap gap-x-5 gap-y-2 pt-1" role="group" aria-label={`Permissions for rule ${index + 1}`}>
            {VERBS.map((v) => (
              <label key={v.bit} class="inline-flex items-center gap-2 text-sm text-fg cursor-pointer">
                <input
                  type="checkbox"
                  class="w-4 h-4 accent-brand cursor-pointer"
                  checked={(rule.mask & v.bit) !== 0}
                  onChange={(e) => onChange({ mask: (e.target as HTMLInputElement).checked ? rule.mask | v.bit : rule.mask & ~v.bit })}
                />
                {v.word}
                <span class="font-mono text-[11px] text-fg-faint">{v.method}</span>
              </label>
            ))}
          </div>
        ) : (
          <p class="text-xs text-fg-muted">
            {rule.level === 'read' && 'List and read rows and table schemas.'}
            {rule.level === 'write' && 'Read, create, replace and update rows. No deletes.'}
            {rule.level === 'full' && 'Everything, including deleting rows.'}
            {rule.level !== 'read' && rule.component.trim() === '' && (
              <> With all tables this also covers creating, altering{rule.level === 'full' ? ' and dropping' : ''} tables. Use <span class="code-chip">_table/*</span> for rows only.</>
            )}
          </p>
        )}
      </div>
    </fieldset>
  );
}

function PatternHelp() {
  return (
    <details class="text-sm text-fg-muted">
      <summary class="cursor-pointer select-none text-fg-muted hover:text-fg w-fit">How matching works</summary>
      <dl class="mt-2 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 pl-1">
        <dt><span class="code-chip">*</span> or empty</dt>
        <dd>Everything: every database, or every table including schema changes.</dd>
        <dt><span class="code-chip">orders</span></dt>
        <dd>The rows and the schema of table orders.</dd>
        <dt><span class="code-chip">_table/orders</span></dt>
        <dd>Only the rows of orders.</dd>
        <dt><span class="code-chip">_table/*</span></dt>
        <dd>Rows of every table and the table list, but no schema changes.</dd>
        <dt><span class="code-chip">prod_*</span></dt>
        <dd>Any database or table whose name starts with prod_.</dd>
      </dl>
      <p class="mt-2">To list a database's tables, a key needs read on all tables or on <span class="code-chip">_table/*</span>.</p>
    </details>
  );
}

function Preview({ form }: { form: RoleForm }) {
  const access = form.rules.map(ruleToAccess);
  if (access.length === 0) {
    return (
      <div class="mt-2">
        <Notice tone="warn">Nothing. Keys with this role are refused on every request.</Notice>
      </div>
    );
  }
  return (
    <div class="mt-2 flex flex-col gap-2">
      {!form.is_active && <Notice tone="warn">This role is inactive, so for now every key with it is refused.</Notice>}
      <ul class="flex flex-col gap-1 text-sm text-fg-muted">
        {access.map((a, i) => (
          <li key={i} class="flex gap-2">
            <Icon name={a.verb_mask === 0 ? 'alert' : 'check'} size={14} class={`mt-[3px] shrink-0 ${a.verb_mask === 0 ? 'text-warn' : 'text-ok'}`} />
            <span>{ruleSentence(a)}</span>
          </li>
        ))}
      </ul>
      <p class="text-xs text-fg-faint">Databases marked read-only refuse writes for every key, whatever the role says.</p>
    </div>
  );
}
