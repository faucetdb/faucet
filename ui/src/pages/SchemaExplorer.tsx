import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { CodeBlock } from '../components/CodeBlock';
import { ServiceRecord } from '../components/ConnectionForm';
import { Icon } from '../components/Icon';
import { confirm, toast } from '../components/Overlay';
import {
  Button,
  ButtonLink,
  CopyButton,
  EmptyState,
  IconButton,
  Input,
  Notice,
  PageHeader,
  Panel,
  Segmented,
  Select,
  Skeleton,
  Spinner,
  Tabs,
  Tag,
  useId,
} from '../components/ui';
import { apiFetch, ApiError, errorMessage } from '../hooks/useApi';
import { engineForDriver } from '../lib/drivers';
import { plural, serverOrigin, shellQuote, timeAgo } from '../lib/format';

/* ------------------------------------------------------------------ Types
   Mirrors internal/model/schema.go and internal/contract/types.go. Go nil
   slices arrive as null, so every list is optional. */

interface Column {
  name: string;
  position: number;
  db_type: string;
  json_type?: string;
  nullable: boolean;
  default?: string | null;
  max_length?: number | null;
  is_primary_key: boolean;
  is_auto_increment: boolean;
  is_unique: boolean;
  comment?: string;
}

interface ForeignKey {
  name: string;
  column_name: string;
  referenced_table: string;
  referenced_column: string;
  on_delete?: string;
  on_update?: string;
}

interface Index {
  name: string;
  columns: string[] | null;
  is_unique: boolean;
}

interface TableSchema {
  name: string;
  type: string; // "table" | "view"
  columns: Column[] | null;
  primary_key: string[] | null;
  foreign_keys: ForeignKey[] | null;
  indexes: Index[] | null;
  row_count?: number;
}

interface SchemaResponse {
  tables: TableSchema[] | null;
  views: TableSchema[] | null;
}

interface Contract {
  table_name: string;
  locked_at: string;
  promoted_at?: string | null;
}

interface DriftItem {
  type: 'additive' | 'breaking';
  category: string;
  table_name: string;
  column_name?: string;
  old_value?: string;
  new_value?: string;
  description: string;
}

interface TableDrift {
  table_name: string;
  has_drift: boolean;
  has_breaking: boolean;
  additive_count: number;
  breaking_count: number;
  items: DriftItem[] | null;
}

interface ServiceDrift {
  service_name: string;
  lock_mode: string;
  total_tables: number;
  drifted_tables: number;
  breaking_count: number;
  tables: TableDrift[] | null;
}

type LockMode = 'none' | 'auto' | 'strict';
type Tab = 'columns' | 'data';

const MODE_OPTIONS: { value: LockMode; label: string }[] = [
  { value: 'none', label: 'Off' },
  { value: 'auto', label: 'Auto' },
  { value: 'strict', label: 'Strict' },
];

const MODE_HELP: Record<LockMode, string> = {
  none: 'The API and its OpenAPI spec follow the live schema. Locked tables still report drift here.',
  auto: 'Locked tables keep their shape in the OpenAPI spec. Columns can be added through Faucet; drops, renames and type changes are blocked.',
  strict: 'No schema change to a locked table goes through Faucet until you promote it.',
};

const DRIFT_LABEL: Record<string, string> = {
  column_added: 'Added since lock',
  column_removed: 'Removed',
  column_renamed: 'Renamed',
  type_changed: 'Type changed',
  nullable_changed: 'Nullability changed',
  table_removed: 'Table removed',
};

const PAGE_SIZE = 25;
/** Identifier shape the record API accepts in ?order= (internal/query/sanitizer.go identifierRegex). */
const SORTABLE = /^[A-Za-z_][A-Za-z0-9_]*$/;
/** SQL keywords the server rejects as identifiers (internal/query/sanitizer.go sqlReservedWords). */
const SQL_RESERVED_WORDS = new Set([
  'SELECT', 'INSERT', 'UPDATE', 'DELETE', 'DROP', 'CREATE', 'ALTER', 'TRUNCATE',
  'EXEC', 'EXECUTE', 'UNION', 'INTO', 'FROM', 'WHERE', 'TABLE', 'DATABASE',
  'GRANT', 'REVOKE', 'INDEX', 'VIEW', 'PROCEDURE', 'FUNCTION', 'TRIGGER', 'SCHEMA',
]);

/** Whether ?order= on this column passes the server's ValidateIdentifier. */
function isSortable(name: string): boolean {
  return name.length <= 128 && SORTABLE.test(name) && !SQL_RESERVED_WORDS.has(name.toUpperCase());
}

const enc = encodeURIComponent;

function asMode(v?: string | null): LockMode {
  return v === 'auto' || v === 'strict' ? v : 'none';
}

/** timeAgo() for mid-sentence use: "just now", "3 min ago" or a date. */
function when(iso: string): string {
  const t = timeAgo(iso);
  return t === 'Just now' ? 'just now' : t;
}

function syncUrl(service: string, table: string | null) {
  const p = new URLSearchParams();
  if (service) p.set('service', service);
  if (table) p.set('table', table);
  const qs = p.toString();
  window.history.replaceState(null, '', `/schema${qs ? `?${qs}` : ''}`);
}

/* ------------------------------------------------------------------- Page */

export function SchemaExplorer() {
  const initial = useRef(new URLSearchParams(window.location.search));
  const pendingTable = useRef<string | null>(initial.current.get('table'));

  const [services, setServices] = useState<ServiceRecord[] | null>(null);
  const [servicesError, setServicesError] = useState<string | null>(null);
  const [service, setService] = useState('');

  const [tables, setTables] = useState<TableSchema[] | null>(null);
  const [schemaError, setSchemaError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const [selected, setSelected] = useState<string | null>(null);
  const [filter, setFilter] = useState('');
  const [tab, setTab] = useState<Tab>('columns');

  const [contracts, setContracts] = useState<Map<string, Contract>>(new Map());
  const [drift, setDrift] = useState<ServiceDrift | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const serviceRef = useRef('');
  serviceRef.current = service;

  /* Databases */
  useEffect(() => {
    apiFetch('/api/v1/system/service')
      .then((res) => {
        const list: ServiceRecord[] = res.resource || [];
        setServices(list);
        const wanted = initial.current.get('service');
        const pick = list.find((s) => s.name === wanted) || list.find((s) => s.is_active) || list[0];
        if (pick) setService(pick.name);
        if (wanted && pick && pick.name !== wanted) pendingTable.current = null;
      })
      .catch((err) => {
        setServicesError(errorMessage(err));
        setServices([]);
      });
  }, []);

  const svc = services?.find((s) => s.name === service) || null;
  const mode = asMode(svc?.schema_lock);

  /* Tables for the selected database */
  useEffect(() => {
    if (!service) return;
    let alive = true;
    setTables(null);
    setSchemaError(null);
    apiFetch<SchemaResponse>(`/api/v1/${enc(service)}/_schema`)
      .then((res) => {
        if (!alive) return;
        const all = [...(res.tables || []), ...(res.views || [])].sort((a, b) => a.name.localeCompare(b.name));
        setTables(all);
        const want = pendingTable.current ?? selected;
        pendingTable.current = null;
        const next = (want && all.find((t) => t.name === want)?.name) || all[0]?.name || null;
        setSelected(next);
        syncUrl(service, next);
        // Listing the schema can auto-lock tables in auto/strict mode.
        loadContracts(service);
      })
      .catch((err) => {
        if (!alive) return;
        setSchemaError(errorMessage(err));
        setTables([]);
      });
    loadContracts(service);
    return () => {
      alive = false;
    };
  }, [service, reloadKey]);

  async function loadContracts(name = service) {
    if (!name) return;
    const [list, diff] = await Promise.allSettled([
      apiFetch(`/api/v1/system/contract/${enc(name)}`),
      apiFetch<ServiceDrift>(`/api/v1/system/contract/${enc(name)}/diff`),
    ]);
    if (name !== serviceRef.current) return;
    if (list.status === 'fulfilled') {
      setContracts(new Map(((list.value.contracts || []) as Contract[]).map((c) => [c.table_name, c])));
    }
    setDrift(diff.status === 'fulfilled' ? diff.value : null);
  }

  function chooseService(name: string) {
    if (name === service) return;
    setService(name);
    setSelected(null);
    setFilter('');
    setContracts(new Map());
    setDrift(null);
    syncUrl(name, null);
  }

  function chooseTable(name: string) {
    setSelected(name);
    syncUrl(service, name);
  }

  /* Derived */
  const driftByTable = useMemo(() => {
    const m = new Map<string, TableDrift>();
    for (const t of drift?.tables || []) m.set(t.table_name, t);
    return m;
  }, [drift]);

  const visible = useMemo(() => {
    const q = filter.trim().toLowerCase();
    return (tables || []).filter((t) => !q || t.name.toLowerCase().includes(q));
  }, [tables, filter]);

  const tableNames = useMemo(() => new Set((tables || []).map((t) => t.name)), [tables]);
  const current = tables?.find((t) => t.name === selected) || null;
  const lockedCount = (tables || []).filter((t) => contracts.has(t.name)).length;
  const unlockedTables = (tables || []).filter((t) => t.type !== 'view' && !contracts.has(t.name));
  const vanished = (drift?.tables || []).filter((t) => !tableNames.has(t.table_name) && tables !== null);
  const hasViews = (tables || []).some((t) => t.type === 'view');

  /* Contract actions */
  async function setMode(next: LockMode) {
    if (!svc || next === mode) return;
    setBusy('mode');
    try {
      await apiFetch(`/api/v1/system/contract/${enc(service)}/mode`, { method: 'PUT', body: { mode: next } });
      setServices((list) => (list || []).map((s) => (s.name === service ? { ...s, schema_lock: next } : s)));
      toast(`Schema lock set to ${MODE_OPTIONS.find((o) => o.value === next)!.label.toLowerCase()}`);
      await loadContracts();
    } catch (err) {
      toast(errorMessage(err, 'Could not change the lock mode'), 'bad');
    } finally {
      setBusy(null);
    }
  }

  async function lockTable(name: string) {
    setBusy(`lock:${name}`);
    try {
      await apiFetch(`/api/v1/system/contract/${enc(service)}/${enc(name)}`, { method: 'POST' });
      toast(`Locked ${name}`);
      await loadContracts();
    } catch (err) {
      toast(errorMessage(err, `Could not lock ${name}`), 'bad');
    } finally {
      setBusy(null);
    }
  }

  async function unlockTable(name: string) {
    const ok = await confirm({
      title: `Unlock ${name}?`,
      body: 'Faucet stops comparing this table with its locked shape, and its API and OpenAPI spec follow the live schema again. You can lock it again at any time.',
      confirmLabel: 'Unlock table',
      danger: true,
    });
    if (!ok) return;
    setBusy(`lock:${name}`);
    try {
      await apiFetch(`/api/v1/system/contract/${enc(service)}/${enc(name)}`, { method: 'DELETE' });
      toast(`Unlocked ${name}`);
      await loadContracts();
    } catch (err) {
      toast(errorMessage(err, `Could not unlock ${name}`), 'bad');
    } finally {
      setBusy(null);
    }
  }

  async function promoteTable(name: string) {
    const d = driftByTable.get(name);
    const breaking = d?.breaking_count || 0;
    const ok = await confirm({
      title: `Promote changes to ${name}?`,
      body: (
        <>
          The lock is updated to match the live table, so these changes become part of the API contract.
          {breaking > 0 && <> {plural(breaking, 'change')} can break clients that rely on the old shape.</>}
        </>
      ),
      confirmLabel: 'Promote changes',
      danger: breaking > 0,
    });
    if (!ok) return;
    setBusy(`promote:${name}`);
    try {
      await apiFetch(`/api/v1/system/contract/${enc(service)}/${enc(name)}/promote`, { method: 'POST' });
      toast(`Promoted changes to ${name}`);
      await loadContracts();
    } catch (err) {
      toast(errorMessage(err, `Could not promote ${name}`), 'bad');
    } finally {
      setBusy(null);
    }
  }

  async function lockRemaining() {
    setBusy('lock-all');
    try {
      if (contracts.size === 0) {
        // Nothing locked yet: one call snapshots every table.
        const res = await apiFetch(`/api/v1/system/contract/${enc(service)}`, { method: 'POST' });
        toast(`Locked ${plural(res?.locked ?? unlockedTables.length, 'table')}`);
      } else {
        // Lock only the unlocked tables. Locking the whole database again
        // would re-snapshot locked tables and silently accept their drift.
        const queue = unlockedTables.map((t) => t.name);
        let done = 0;
        const failed: string[] = [];
        const worker = async () => {
          for (let name = queue.shift(); name !== undefined; name = queue.shift()) {
            try {
              await apiFetch(`/api/v1/system/contract/${enc(service)}/${enc(name)}`, { method: 'POST' });
              done++;
            } catch {
              failed.push(name);
            }
          }
        };
        await Promise.all([worker(), worker(), worker(), worker()]);
        if (failed.length) toast(`Locked ${plural(done, 'table')}. Could not lock ${failed.join(', ')}`, 'bad');
        else toast(`Locked ${plural(done, 'table')}`);
      }
      await loadContracts();
    } catch (err) {
      toast(errorMessage(err, 'Could not lock the tables'), 'bad');
    } finally {
      setBusy(null);
    }
  }

  async function unlockAll() {
    const ok = await confirm({
      title: `Unlock every table in ${service}?`,
      body: `This removes ${plural(contracts.size, 'lock')}. The API and OpenAPI spec follow the live schema again, and drift is no longer tracked.`,
      confirmLabel: 'Unlock all',
      danger: true,
    });
    if (!ok) return;
    setBusy('unlock-all');
    try {
      const res = await apiFetch(`/api/v1/system/contract/${enc(service)}`, { method: 'DELETE' });
      toast(`Unlocked ${plural(res?.removed ?? contracts.size, 'table')}`);
      await loadContracts();
    } catch (err) {
      toast(errorMessage(err, 'Could not unlock the tables'), 'bad');
    } finally {
      setBusy(null);
    }
  }

  /* Render */
  const pickerId = useId('db');
  const picker =
    services && services.length > 0 ? (
      <div class="flex items-center gap-2">
        <label for={pickerId} class="text-sm text-fg-muted">Database</label>
        <Select
          id={pickerId}
          class="w-56 max-w-[60vw]"
          value={service}
          onValue={chooseService}
          options={services.map((s) => ({
            value: s.name,
            label: `${s.name} (${engineForDriver(s.driver).label}${s.is_active ? '' : ', paused'})`,
          }))}
        />
      </div>
    ) : undefined;

  return (
    <div>
      <PageHeader
        title="Schema"
        description="Browse tables and columns, preview rows, and lock the shape of your API so schema changes can't surprise its clients."
        actions={picker}
      />

      {servicesError && (
        <div class="mb-4">
          <Notice tone="bad" title="Could not load your databases">{servicesError}</Notice>
        </div>
      )}

      {services === null ? (
        <div class="grid gap-6 lg:grid-cols-[280px_minmax(0,1fr)]">
          <Skeleton class="h-80 rounded-[10px]" />
          <Skeleton class="h-80 rounded-[10px]" />
        </div>
      ) : services.length === 0 ? (
        !servicesError && (
          <Panel>
            <EmptyState
              icon="database"
              title="Connect a database to see its schema"
              action={<ButtonLink variant="primary" icon="plus" href="/services?add=1">Add database</ButtonLink>}
            >
              Once a database is connected, its tables, columns and relationships show up here, along with a preview of the data.
            </EmptyState>
          </Panel>
        )
      ) : (
        <div class="flex flex-col gap-6">
          {schemaError ? (
            <Panel>
              <EmptyState
                icon="alert"
                title={`Faucet could not read the schema of ${service}`}
                action={
                  <>
                    <Button icon="refresh" onClick={() => setReloadKey((k) => k + 1)}>Try again</Button>
                    <ButtonLink variant="ghost" href="/services">Check the connection</ButtonLink>
                  </>
                }
              >
                <p class="font-mono text-sm break-all">{schemaError}</p>
                <p class="mt-2">
                  {svc && !svc.is_active
                    ? 'This database is paused. Resume it on the Databases page to browse it.'
                    : 'Make sure the database is reachable and the user can read its catalog.'}
                </p>
              </EmptyState>
            </Panel>
          ) : (
            <>
              <LockPanel
                mode={mode}
                busy={busy}
                onMode={setMode}
                tableCount={(tables || []).length}
                lockedCount={lockedCount}
                contractCount={contracts.size}
                unlockedTableCount={unlockedTables.length}
                drift={drift}
                vanished={vanished}
                onLockRemaining={lockRemaining}
                onUnlockAll={unlockAll}
                onUnlock={unlockTable}
                loading={tables === null}
              />

              {tables !== null && tables.length === 0 ? (
                <Panel>
                  <EmptyState
                    icon="table"
                    title={`No tables found in ${service}`}
                    action={
                      <>
                        <ButtonLink href="/services">Open databases</ButtonLink>
                        <Button variant="ghost" icon="refresh" onClick={() => setReloadKey((k) => k + 1)}>Check again</Button>
                      </>
                    }
                  >
                    Faucet connected but found no tables or views it can read
                    {svc?.schema ? <> in the <span class="font-mono text-fg">{svc.schema}</span> schema</> : null}. If your tables live in another schema, set the
                    schema option on the database (for example <span class="font-mono">public</span> or <span class="font-mono">dbo</span>), and check that the database user can see them.
                  </EmptyState>
                </Panel>
              ) : (
                <div class="grid gap-6 lg:grid-cols-[280px_minmax(0,1fr)] items-start">
                  <TableList
                    tables={tables}
                    visible={visible}
                    filter={filter}
                    onFilter={setFilter}
                    selected={selected}
                    onSelect={chooseTable}
                    contracts={contracts}
                    driftByTable={driftByTable}
                    noun={hasViews ? 'tables and views' : 'tables'}
                  />
                  {tables === null ? (
                    <Panel bodyClass="p-5 flex flex-col gap-3">
                      <Skeleton class="h-5 w-48" />
                      <Skeleton class="h-4 w-72" />
                      <Skeleton class="h-40 w-full" />
                    </Panel>
                  ) : current ? (
                    <TableDetail
                      key={`${service}/${current.name}`}
                      service={service}
                      table={current}
                      contract={contracts.get(current.name) || null}
                      drift={driftByTable.get(current.name) || null}
                      mode={mode}
                      tab={tab}
                      onTab={setTab}
                      tableNames={tableNames}
                      onSelectTable={chooseTable}
                      busy={busy}
                      onLock={lockTable}
                      onUnlock={unlockTable}
                      onPromote={promoteTable}
                    />
                  ) : (
                    <Panel>
                      <EmptyState icon="table" title="Pick a table">
                        Choose a table on the left to see its columns and a preview of its rows.
                      </EmptyState>
                    </Panel>
                  )}
                </div>
              )}
            </>
          )}
        </div>
      )}
    </div>
  );
}

/* ------------------------------------------------------------- Lock panel */

function LockPanel(props: {
  mode: LockMode;
  busy: string | null;
  onMode: (m: LockMode) => void;
  tableCount: number;
  lockedCount: number;
  contractCount: number;
  unlockedTableCount: number;
  drift: ServiceDrift | null;
  vanished: TableDrift[];
  onLockRemaining: () => void;
  onUnlockAll: () => void;
  onUnlock: (name: string) => void;
  loading: boolean;
}) {
  const { mode, busy, drift, lockedCount, tableCount } = props;
  const drifted = drift?.drifted_tables || 0;
  const breaking = drift?.breaking_count || 0;

  return (
    <Panel>
      <div class="flex flex-wrap items-start justify-between gap-x-6 gap-y-4 px-5 py-4">
        <div class="min-w-0 flex-1 basis-80">
          <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
            <h2 class="text-base font-semibold text-fg">Schema lock</h2>
            {!props.loading && (
              <span class="text-sm text-fg-muted">
                {lockedCount === 0 ? 'No tables locked' : `${lockedCount} of ${plural(tableCount, 'table')} locked`}
              </span>
            )}
            {drifted > 0 && (
              <Tag tone={breaking > 0 ? 'bad' : 'warn'}>
                {plural(drifted, 'table')} changed{breaking > 0 ? `, ${plural(breaking, 'breaking change')}` : ''}
              </Tag>
            )}
          </div>
          <p class="text-sm text-fg-muted mt-1 max-w-[70ch]">
            Locking a table saves its current shape as the API contract, so you can see when the database drifts from what clients expect.
          </p>
        </div>
        <div class="flex flex-col items-start gap-2 basis-80 grow sm:grow-0">
          <div class="flex items-center gap-2">
            <Segmented label="Schema lock mode" value={mode} onChange={props.onMode} options={MODE_OPTIONS} />
            {busy === 'mode' && <Spinner size={14} class="text-fg-muted" />}
          </div>
          <p class="text-sm text-fg-muted max-w-[52ch]" aria-live="polite">
            {MODE_HELP[mode]}
            {mode === 'none' && props.contractCount === 0 && ' Turning it on locks every table now.'}
          </p>
        </div>
      </div>

      {!props.loading && (props.unlockedTableCount > 0 || props.contractCount > 0) && (
        <div class="flex flex-wrap items-center gap-2 px-5 py-3 border-t border-line">
          {props.unlockedTableCount > 0 && (
            <Button size="sm" icon="lock" loading={busy === 'lock-all'} disabled={!!busy && busy !== 'lock-all'} onClick={props.onLockRemaining}>
              {props.contractCount === 0 ? 'Lock all tables' : `Lock ${plural(props.unlockedTableCount, 'remaining table')}`}
            </Button>
          )}
          {props.contractCount > 0 && (
            <Button size="sm" variant="ghost" icon="unlock" loading={busy === 'unlock-all'} disabled={!!busy && busy !== 'unlock-all'} onClick={props.onUnlockAll}>
              Unlock all
            </Button>
          )}
        </div>
      )}

      {props.vanished.length > 0 && (
        <div class="px-5 py-3 border-t border-line">
          <Notice tone="bad" title="Locked tables are missing from the database">
            <ul class="mt-1 flex flex-col gap-1.5">
              {props.vanished.map((t) => (
                <li key={t.table_name} class="flex flex-wrap items-center gap-2">
                  <span class="font-mono text-fg">{t.table_name}</span>
                  <span>was dropped or renamed. Clients using it will get errors.</span>
                  <Button size="sm" variant="ghost" onClick={() => props.onUnlock(t.table_name)}>Unlock table</Button>
                </li>
              ))}
            </ul>
          </Notice>
        </div>
      )}
    </Panel>
  );
}

/* ------------------------------------------------------------- Table list */

function TableList(props: {
  tables: TableSchema[] | null;
  visible: TableSchema[];
  filter: string;
  onFilter: (v: string) => void;
  selected: string | null;
  onSelect: (name: string) => void;
  contracts: Map<string, Contract>;
  driftByTable: Map<string, TableDrift>;
  noun: string;
}) {
  const { tables, visible } = props;
  const filterId = useId('tf');

  return (
    <Panel class="min-w-0 lg:sticky lg:top-6" bodyClass="flex flex-col">
      <div class="p-3 border-b border-line">
        <label for={filterId} class="sr-only">Filter tables</label>
        <div class="relative">
          <Icon name="search" size={15} class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-fg-faint" />
          <Input
            id={filterId}
            type="search"
            class="pl-8"
            placeholder="Filter tables"
            value={props.filter}
            onValue={props.onFilter}
            autoComplete="off"
            spellcheck={false}
          />
        </div>
      </div>

      <div class="max-h-[280px] lg:max-h-[calc(100vh-280px)] overflow-y-auto">
        {tables === null ? (
          <div class="p-3 flex flex-col gap-2">
            {[0, 1, 2, 3, 4, 5].map((i) => (
              <Skeleton key={i} class="h-6 w-full" />
            ))}
          </div>
        ) : visible.length === 0 ? (
          <div class="px-4 py-6 text-sm text-fg-muted">
            No tables match <span class="font-mono text-fg">{props.filter}</span>.{' '}
            <button type="button" class="link" onClick={() => props.onFilter('')}>Clear the filter</button>
          </div>
        ) : (
          <ul class="p-1.5" aria-label="Tables">
            {visible.map((t) => {
              const active = t.name === props.selected;
              const locked = props.contracts.has(t.name);
              const d = props.driftByTable.get(t.name);
              return (
                <li key={t.name}>
                  <button
                    type="button"
                    onClick={() => props.onSelect(t.name)}
                    aria-current={active ? 'true' : undefined}
                    class={`w-full flex items-center gap-2 h-8 px-2.5 rounded-[6px] text-left transition-colors ${
                      active ? 'bg-brand/10 text-fg' : 'text-fg-muted hover:bg-panel-2 hover:text-fg'
                    }`}
                  >
                    <Icon name={t.type === 'view' ? 'eye' : 'table'} size={14} class={active ? 'text-brand-fg' : 'text-fg-faint'} />
                    <span class="flex-1 min-w-0 truncate font-mono text-[13px]" title={t.name}>{t.name}</span>
                    {t.type === 'view' && <span class="text-xs text-fg-faint">View</span>}
                    {locked && <Icon name="lock" size={13} class="text-fg-faint" label="Locked" />}
                    {locked && <DriftDot drift={d} />}
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </div>

      {tables && tables.length > 0 && (
        <div class="flex items-center justify-between gap-2 px-4 py-2.5 border-t border-line text-xs text-fg-muted">
          <span>
            {visible.length === tables.length
              ? tables.length === 1 && props.noun === 'tables'
                ? '1 table'
                : `${tables.length} ${props.noun}`
              : `${visible.length} of ${tables.length} ${props.noun}`}
          </span>
          {props.contracts.size > 0 && <span>{props.contracts.size} locked</span>}
        </div>
      )}
    </Panel>
  );
}

function DriftDot({ drift }: { drift?: TableDrift }) {
  const state = !drift || !drift.has_drift ? 'ok' : drift.has_breaking ? 'bad' : 'warn';
  const label = state === 'ok' ? 'Matches its lock' : state === 'bad' ? 'Breaking changes since lock' : 'Additive changes since lock';
  const color = state === 'ok' ? 'bg-ok' : state === 'bad' ? 'bg-bad' : 'bg-warn';
  return (
    <span class="inline-flex items-center" title={label}>
      <span class={`w-2 h-2 rounded-full ${color}`} aria-hidden="true" />
      <span class="sr-only">{label}</span>
    </span>
  );
}

/* ----------------------------------------------------------- Table detail */

function TableDetail(props: {
  service: string;
  table: TableSchema;
  contract: Contract | null;
  drift: TableDrift | null;
  mode: LockMode;
  tab: Tab;
  onTab: (t: Tab) => void;
  tableNames: Set<string>;
  onSelectTable: (name: string) => void;
  busy: string | null;
  onLock: (name: string) => void;
  onUnlock: (name: string) => void;
  onPromote: (name: string) => void;
}) {
  const { service, table, contract, drift, busy } = props;
  const columns = table.columns || [];
  const endpoint = `${serverOrigin()}/api/v1/${enc(service)}/_table/${enc(table.name)}`;
  const isView = table.type === 'view';
  const driftItems = drift?.items || [];
  const lockBusy = busy === `lock:${table.name}`;
  const promoteBusy = busy === `promote:${table.name}`;

  const lockedSince = contract
    ? contract.promoted_at && Math.abs(new Date(contract.promoted_at).getTime() - new Date(contract.locked_at).getTime()) > 1000
      ? `Locked ${when(contract.locked_at)}, last promoted ${when(contract.promoted_at)}.`
      : `Locked ${when(contract.locked_at)}.`
    : null;

  return (
    <section class="min-w-0 bg-panel border border-line rounded-[var(--radius-panel)]">
      <header class="flex flex-wrap items-start justify-between gap-x-4 gap-y-3 px-5 pt-4 pb-3">
        <div class="min-w-0 flex-1 basis-64">
          <div class="flex flex-wrap items-center gap-x-2.5 gap-y-1">
            <h2 class="text-lg font-semibold text-fg font-mono break-all">{table.name}</h2>
            {isView && <Tag>View</Tag>}
            {contract && <Tag>Locked</Tag>}
            {contract && drift?.has_drift && <Tag tone={drift.has_breaking ? 'bad' : 'warn'}>Changed since lock</Tag>}
          </div>
          <p class="text-sm text-fg-muted mt-0.5">
            {plural(columns.length, 'column')}
            {table.row_count != null && <>, about {plural(table.row_count, 'row')}</>}.
            {lockedSince && <> {lockedSince}</>}
          </p>
          <div class="flex items-center gap-1 mt-2 -ml-0.5 min-w-0">
            <code class="text-[12.5px] text-fg-muted truncate" title={endpoint}>{endpoint}</code>
            <CopyButton text={endpoint} label="Copy URL" />
          </div>
        </div>
        <div class="flex flex-wrap items-center gap-1.5">
          <ButtonLink size="sm" icon="terminal" href={`/api-explorer?service=${enc(service)}&table=${enc(table.name)}`}>
            Query in API explorer
          </ButtonLink>
          {contract && drift?.has_drift && (
            <Button size="sm" variant="primary" icon="check" loading={promoteBusy} disabled={!!busy && !promoteBusy} onClick={() => props.onPromote(table.name)}>
              Promote changes
            </Button>
          )}
          {contract ? (
            <Button size="sm" variant="ghost" icon="unlock" loading={lockBusy} disabled={!!busy && !lockBusy} onClick={() => props.onUnlock(table.name)}>
              Unlock
            </Button>
          ) : (
            <Button size="sm" icon="lock" loading={lockBusy} disabled={!!busy && !lockBusy} onClick={() => props.onLock(table.name)}>
              Lock table
            </Button>
          )}
        </div>
      </header>

      {contract && drift?.has_drift && driftItems.length > 0 && (
        <div class="px-5 pb-3">
          <Notice
            tone={drift.has_breaking ? 'bad' : 'warn'}
            title={drift.has_breaking ? 'The live table no longer matches its lock' : 'The live table has new changes since it was locked'}
          >
            <ul class="mt-1 flex flex-col gap-1.5">
              {driftItems.map((item, i) => (
                <li key={i} class="flex flex-wrap items-center gap-2">
                  <Tag tone={item.type === 'breaking' ? 'bad' : 'warn'}>{item.type === 'breaking' ? 'Breaking' : 'Additive'}</Tag>
                  <span class="text-fg">{item.description}</span>
                </li>
              ))}
            </ul>
            <p class="mt-2">
              Promote the changes to make them part of the contract
              {props.mode === 'none' ? '. The lock mode is off, so nothing is blocked right now.' : ', or revert them in the database.'}
            </p>
          </Notice>
        </div>
      )}

      <Tabs
        class="px-3"
        value={props.tab}
        onChange={props.onTab}
        tabs={[
          { value: 'columns', label: <>Columns <span class="text-fg-faint font-normal">{columns.length}</span></> },
          { value: 'data', label: 'Data' },
        ]}
      />

      {props.tab === 'columns' ? (
        <ColumnsView table={table} drift={contract ? driftItems : []} tableNames={props.tableNames} onSelectTable={props.onSelectTable} />
      ) : (
        <DataView service={service} table={table} endpoint={endpoint} />
      )}
    </section>
  );
}

/* --------------------------------------------------------------- Columns */

function ColumnsView({ table, drift, tableNames, onSelectTable }: {
  table: TableSchema;
  drift: DriftItem[];
  tableNames: Set<string>;
  onSelectTable: (name: string) => void;
}) {
  const columns = [...(table.columns || [])].sort((a, b) => (a.position || 0) - (b.position || 0));
  const pk = new Set(table.primary_key || []);
  const fks = new Map<string, ForeignKey[]>();
  for (const fk of table.foreign_keys || []) fks.set(fk.column_name, [...(fks.get(fk.column_name) || []), fk]);
  const driftByCol = new Map<string, DriftItem[]>();
  for (const d of drift) if (d.column_name) driftByCol.set(d.column_name, [...(driftByCol.get(d.column_name) || []), d]);
  const indexes = table.indexes || [];

  if (columns.length === 0) {
    return <p class="px-5 py-6 text-sm text-fg-muted">This {table.type === 'view' ? 'view' : 'table'} has no columns Faucet can read.</p>;
  }

  return (
    <div>
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="border-b border-line text-left text-fg-muted">
              <th scope="col" class="px-5 py-2.5 font-medium">Column</th>
              <th scope="col" class="px-3 py-2.5 font-medium">Type</th>
              <th scope="col" class="px-3 py-2.5 font-medium">Nullable</th>
              <th scope="col" class="px-3 py-2.5 font-medium">Default</th>
              <th scope="col" class="px-3 pr-5 py-2.5 font-medium">Keys and notes</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-line">
            {columns.map((col) => {
              const isPk = col.is_primary_key || pk.has(col.name);
              const colFks = fks.get(col.name) || [];
              const colDrift = driftByCol.get(col.name) || [];
              return (
                <tr key={col.name} class="align-top hover:bg-panel-2/50">
                  <td class="px-5 py-2.5">
                    <span class="font-mono text-[13px] text-fg break-all">{col.name}</span>
                    {col.comment && <p class="text-xs text-fg-muted mt-0.5 max-w-[40ch]">{col.comment}</p>}
                  </td>
                  <td class="px-3 py-2.5 font-mono text-[12.5px] text-fg-muted whitespace-nowrap">{col.db_type}</td>
                  <td class="px-3 py-2.5 whitespace-nowrap">{col.nullable ? <span class="text-fg-muted">Yes</span> : <span class="text-fg">No</span>}</td>
                  <td class="px-3 py-2.5">
                    {col.default != null && col.default !== '' ? (
                      <span class="font-mono text-[12.5px] text-fg-muted break-all line-clamp-2" title={col.default}>{col.default}</span>
                    ) : (
                      <span class="text-fg-faint" aria-label="None">-</span>
                    )}
                  </td>
                  <td class="px-3 pr-5 py-2">
                    <div class="flex flex-wrap items-center gap-1.5">
                      {isPk && <Tag tone="brand">Primary key</Tag>}
                      {col.is_auto_increment && <Tag>Auto-increment</Tag>}
                      {col.is_unique && !isPk && <Tag>Unique</Tag>}
                      {colFks.map((fk) => {
                        const target = `${fk.referenced_table}.${fk.referenced_column}`;
                        return tableNames.has(fk.referenced_table) ? (
                          <button
                            key={fk.name || target}
                            type="button"
                            onClick={() => onSelectTable(fk.referenced_table)}
                            title={`Open ${fk.referenced_table}`}
                            class="inline-flex items-center gap-1 h-5 px-1.5 rounded text-xs font-medium border border-brand/20 bg-brand/10 text-brand-fg hover:bg-brand/20 transition-colors"
                          >
                            <Icon name="link" size={12} />
                            References <span class="font-mono">{target}</span>
                          </button>
                        ) : (
                          <Tag key={fk.name || target}>
                            References <span class="font-mono ml-1">{target}</span>
                          </Tag>
                        );
                      })}
                      {colDrift.map((d, i) => (
                        <span key={i} title={d.description}>
                          <Tag tone={d.type === 'breaking' ? 'bad' : 'warn'}>{DRIFT_LABEL[d.category] || 'Changed since lock'}</Tag>
                        </span>
                      ))}
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {indexes.length > 0 && (
        <div class="border-t border-line">
          <h3 class="px-5 pt-3 pb-1 text-sm font-medium text-fg">Indexes</h3>
          <ul class="pb-2">
            {indexes.map((ix) => (
              <li key={ix.name} class="flex flex-wrap items-center gap-x-3 gap-y-1 px-5 py-1.5 text-sm">
                <span class="font-mono text-[13px] text-fg break-all">{ix.name}</span>
                <span class="font-mono text-[12.5px] text-fg-muted break-all">{(ix.columns || []).join(', ')}</span>
                {ix.is_unique && <Tag>Unique</Tag>}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

/* ------------------------------------------------------------------ Data */

type Row = Record<string, unknown>;
interface Sort {
  col: string;
  dir: 'asc' | 'desc';
}

function DataView({ service, table, endpoint }: { service: string; table: TableSchema; endpoint: string }) {
  const [rows, setRows] = useState<Row[] | null>(null);
  const [total, setTotal] = useState<number | null>(null);
  const [lastPageFull, setLastPageFull] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sort, setSort] = useState<Sort | null>(null);
  const seq = useRef(0);

  // A single-column primary key gives stable pages when nothing else is sorted.
  const pkCols = table.primary_key?.length ? table.primary_key : (table.columns || []).filter((c) => c.is_primary_key).map((c) => c.name);
  const defaultOrder = pkCols.length === 1 && isSortable(pkCols[0]) ? `${pkCols[0]} ASC` : '';

  function orderFor(s: Sort | null) {
    return s ? `${s.col} ${s.dir.toUpperCase()}` : defaultOrder;
  }

  async function load(offset: number, s: Sort | null, append: boolean) {
    const id = ++seq.current;
    setLoading(true);
    setError(null);
    const fetchPage = (order: string) => {
      const p = new URLSearchParams({ limit: String(PAGE_SIZE), offset: String(offset), include_count: 'true' });
      if (order) p.set('order', order);
      return apiFetch(`/api/v1/${enc(service)}/_table/${enc(table.name)}?${p}`);
    };
    try {
      let res;
      try {
        res = await fetchPage(orderFor(s));
      } catch (err) {
        // The default primary-key order can be rejected (e.g. a reserved
        // word); fall back to the database's natural order.
        if (!s && defaultOrder && err instanceof ApiError && err.status === 400) res = await fetchPage('');
        else throw err;
      }
      if (id !== seq.current) return;
      const page: Row[] = res?.resource || [];
      setRows((prev) => (append && prev ? [...prev, ...page] : page));
      setTotal(typeof res?.meta?.total === 'number' ? res.meta.total : null);
      setLastPageFull(page.length === PAGE_SIZE);
    } catch (err) {
      if (id !== seq.current) return;
      setError(errorMessage(err, 'The rows could not be loaded.'));
      if (!append) setRows([]);
    } finally {
      if (id === seq.current) setLoading(false);
    }
  }

  useEffect(() => {
    load(0, null, false);
  }, []);

  function toggleSort(col: string) {
    const next: Sort | null = !sort || sort.col !== col ? { col, dir: 'asc' } : sort.dir === 'asc' ? { col, dir: 'desc' } : null;
    setSort(next);
    load(0, next, false);
  }

  const columnNames = useMemo(() => {
    const names = [...(table.columns || [])].sort((a, b) => (a.position || 0) - (b.position || 0)).map((c) => c.name);
    const seen = new Set(names);
    for (const r of rows || []) {
      for (const k of Object.keys(r)) {
        if (!seen.has(k)) {
          seen.add(k);
          names.push(k);
        }
      }
    }
    return names;
  }, [table, rows]);

  const count = rows?.length || 0;
  const hasMore = total != null ? count < total : lastPageFull;
  const order = orderFor(sort);
  const curl = `curl -H "X-API-Key: $FAUCET_API_KEY" \\\n  ${shellQuote(`${endpoint}?limit=${PAGE_SIZE}${order ? `&order=${enc(order)}` : ''}`)}`;

  if (rows === null) {
    return (
      <div class="px-5 py-8 flex items-center gap-2 text-sm text-fg-muted" role="status">
        <Spinner size={14} /> Loading rows
      </div>
    );
  }

  return (
    <div>
      {error && (
        <div class="px-5 pt-4">
          <Notice tone="bad" title="Could not load rows">
            <p class="font-mono text-xs break-all">{error}</p>
            <p class="mt-1">Check that the database is reachable, then try again.</p>
          </Notice>
        </div>
      )}

      {count === 0 && !error ? (
        <p class="px-5 py-6 text-sm text-fg-muted">
          {table.type === 'view' ? 'This view returns no rows.' : 'This table is empty. Insert rows with a POST to its endpoint, or from the API explorer.'}
        </p>
      ) : count > 0 ? (
        <div class="overflow-x-auto max-h-[60vh] overflow-y-auto">
          <table class="min-w-full text-sm">
            <thead class="sticky top-0 bg-panel z-[1]">
              <tr class="border-b border-line text-left">
                {columnNames.map((name) => {
                  const sortable = isSortable(name);
                  const active = sort?.col === name;
                  return (
                    <th
                      key={name}
                      scope="col"
                      aria-sort={active ? (sort!.dir === 'asc' ? 'ascending' : 'descending') : undefined}
                      class="px-3 first:pl-5 py-2 font-medium whitespace-nowrap"
                    >
                      {sortable ? (
                        <button
                          type="button"
                          onClick={() => toggleSort(name)}
                          title={`Sort by ${name}`}
                          class={`inline-flex items-center gap-1 font-mono text-[12.5px] hover:text-fg ${active ? 'text-fg' : 'text-fg-muted'}`}
                        >
                          {name}
                          {active && <Icon name="chevronDown" size={12} class={sort!.dir === 'asc' ? 'rotate-180' : ''} />}
                        </button>
                      ) : (
                        <span class="font-mono text-[12.5px] text-fg-muted">{name}</span>
                      )}
                    </th>
                  );
                })}
              </tr>
            </thead>
            <tbody class="divide-y divide-line">
              {rows.map((row, i) => (
                <tr key={i} class="hover:bg-panel-2/50">
                  {columnNames.map((name) => (
                    <Cell key={name} value={row[name]} />
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      <div class="flex flex-wrap items-center gap-x-3 gap-y-2 px-5 py-3 border-t border-line">
        <span class="text-sm text-fg-muted" aria-live="polite">
          {total != null
            ? `Showing ${count.toLocaleString()} of ${total.toLocaleString()} ${total === 1 ? 'row' : 'rows'}`
            : `Showing ${plural(count, 'row')}`}
        </span>
        <span class="flex-1" />
        {hasMore && (
          <Button size="sm" loading={loading && count > 0} onClick={() => load(count, sort, true)}>
            Load {PAGE_SIZE} more
          </Button>
        )}
        <IconButton icon="refresh" label="Reload rows" onClick={() => load(0, sort, false)} />
      </div>

      <div class="px-5 pb-5 pt-1">
        <p class="text-sm text-fg-muted mb-2">
          The same rows from the API. Set <span class="font-mono">FAUCET_API_KEY</span> to a key from the <a href="/api-keys" class="link">API keys</a> page.
        </p>
        <CodeBlock title="curl" code={curl} />
      </div>
    </div>
  );
}

function Cell({ value }: { value: unknown }) {
  if (value === null || value === undefined) {
    return <td class="px-3 first:pl-5 py-1.5 font-mono text-[12.5px] text-fg-faint italic whitespace-nowrap">null</td>;
  }
  const text = typeof value === 'object' ? JSON.stringify(value) : String(value);
  const long = text.length > 40;
  return (
    <td class="px-3 first:pl-5 py-1.5 font-mono text-[12.5px] text-fg whitespace-nowrap">
      <span class="block max-w-[280px] truncate" title={long ? text : undefined}>{text}</span>
    </td>
  );
}
