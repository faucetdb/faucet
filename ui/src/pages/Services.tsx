import { useEffect, useState } from 'preact/hooks';
import { DatabaseDrawer } from '../components/DatabaseDrawer';
import { EnginePicker, ServiceRecord } from '../components/ConnectionForm';
import { DbLogo } from '../components/DbLogo';
import { Icon } from '../components/Icon';
import { confirm, toast } from '../components/Overlay';
import { Button, ButtonLink, CopyButton, EmptyState, PageHeader, Panel, Skeleton, StatusDot, Tag } from '../components/ui';
import { apiFetch, errorMessage } from '../hooks/useApi';
import { EngineId, describeConnection, engineForDriver } from '../lib/drivers';
import { serverOrigin } from '../lib/format';
import { useHealth } from '../lib/server';

export function Services() {
  const [services, setServices] = useState<ServiceRecord[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [drawer, setDrawer] = useState<{ open: boolean; editing?: ServiceRecord | null; engine?: EngineId }>({ open: false });
  const [testing, setTesting] = useState<string | null>(null);
  const [rowResult, setRowResult] = useState<Record<string, { ok: boolean; message: string }>>({});
  const [flash, setFlash] = useState<string | null>(null);
  const { checks, refresh: refreshHealth } = useHealth(15_000);

  useEffect(() => {
    load();
    // Deep link from the overview: /services?add=1 opens the drawer.
    if (new URLSearchParams(window.location.search).get('add')) {
      setDrawer({ open: true });
      window.history.replaceState(null, '', '/services');
    }
  }, []);

  async function load() {
    try {
      const res = await apiFetch('/api/v1/system/service');
      setServices(res.resource || []);
      setLoadError(null);
    } catch (err) {
      setLoadError(errorMessage(err));
      setServices([]);
    }
  }

  async function test(name: string) {
    setTesting(name);
    try {
      await apiFetch(`/api/v1/system/service/${encodeURIComponent(name)}/test`);
      setRowResult((r) => ({ ...r, [name]: { ok: true, message: 'Connection works' } }));
      refreshHealth();
    } catch (err) {
      setRowResult((r) => ({ ...r, [name]: { ok: false, message: errorMessage(err, 'Connection failed') } }));
    } finally {
      setTesting(null);
    }
  }

  async function remove(svc: ServiceRecord) {
    const ok = await confirm({
      title: `Remove ${svc.name}?`,
      body: (
        <>
          Its API at <span class="font-mono text-fg">/api/v1/{svc.name}</span> and its MCP tools stop working right away. The database itself is not touched.
        </>
      ),
      confirmLabel: 'Remove database',
      danger: true,
    });
    if (!ok) return;
    try {
      await apiFetch(`/api/v1/system/service/${encodeURIComponent(svc.name)}`, { method: 'DELETE' });
      toast(`Removed ${svc.name}`);
      load();
    } catch (err) {
      toast(errorMessage(err, 'Could not remove the database'), 'bad');
    }
  }

  async function setActive(svc: ServiceRecord, active: boolean) {
    try {
      await apiFetch(`/api/v1/system/service/${encodeURIComponent(svc.name)}`, {
        method: 'PUT',
        body: { is_active: active, read_only: svc.read_only, raw_sql_allowed: svc.raw_sql_allowed },
      });
      toast(active ? `Resumed ${svc.name}` : `Paused ${svc.name}`);
      load();
      refreshHealth();
    } catch (err) {
      toast(errorMessage(err), 'bad');
    }
  }

  function onSaved(svc: ServiceRecord, warning?: string) {
    const wasEditing = !!drawer.editing;
    setDrawer({ open: false });
    setFlash(svc.name);
    setRowResult((r) => {
      const { [svc.name]: _, ...rest } = r;
      return warning ? { ...rest, [svc.name]: { ok: false, message: warning } } : rest;
    });
    if (warning) toast(`Saved ${svc.name}, but it is not connected yet`, 'bad');
    else toast(wasEditing ? `Saved changes to ${svc.name}` : `Added ${svc.name}. Its API is live at /api/v1/${svc.name}`);
    load();
    refreshHealth();
  }

  function status(svc: ServiceRecord) {
    if (!svc.is_active) return <StatusDot status="idle" label="Paused" />;
    const check = checks[svc.name];
    if (check === 'ok') return <StatusDot status="ok" label="Connected" />;
    if (check) return <StatusDot status="bad" label="Unreachable" />;
    return <StatusDot status="warn" label="Not connected" />;
  }

  const addButton = (
    <Button variant="primary" icon="plus" onClick={() => setDrawer({ open: true })}>
      Add database
    </Button>
  );

  return (
    <div>
      <PageHeader
        title="Databases"
        description="Each database you connect gets a REST API and MCP tools for AI agents."
        actions={services && services.length > 0 ? addButton : undefined}
      />

      {loadError && <p class="mb-4 text-sm text-bad">Could not load databases: {loadError}</p>}

      {services === null ? (
        <Panel bodyClass="divide-y divide-line">
          {[0, 1].map((i) => (
            <div key={i} class="flex items-center gap-4 px-5 py-4">
              <Skeleton class="w-9 h-9 rounded-[8px]" />
              <div class="flex-1 flex flex-col gap-2">
                <Skeleton class="h-4 w-40" />
                <Skeleton class="h-3 w-64" />
              </div>
            </div>
          ))}
        </Panel>
      ) : services.length === 0 ? (
        <Panel>
          <EmptyState icon="database" title="Connect your first database">
            Pick what you're running. You'll enter the host, port, user and password, and Faucet does the rest.
          </EmptyState>
          <div class="px-6 pb-8 sm:px-10 -mt-2">
            <EnginePicker value={'' as EngineId} onChange={(engine) => setDrawer({ open: true, engine })} />
          </div>
        </Panel>
      ) : (
        <Panel bodyClass="divide-y divide-line">
          {services.map((svc) => {
            const engine = engineForDriver(svc.driver);
            const where = describeConnection(svc.driver, svc.connection);
            const endpoint = `${serverOrigin()}/api/v1/${svc.name}`;
            const result = rowResult[svc.name];
            return (
              <div key={svc.name} class={`px-5 py-4 ${flash === svc.name ? 'anim-flash' : ''}`}>
                <div class="flex flex-wrap items-start gap-x-4 gap-y-3">
                  <DbLogo driver={svc.driver} size={36} />
                  <div class="min-w-0 flex-1 basis-60">
                    <div class="flex flex-wrap items-center gap-x-2.5 gap-y-1">
                      <h2 class="text-base font-semibold text-fg font-mono">{svc.name}</h2>
                      {status(svc)}
                      {svc.read_only && <Tag>Read-only</Tag>}
                      {svc.raw_sql_allowed && <Tag tone="warn">Raw SQL on</Tag>}
                    </div>
                    <p class="text-sm text-fg-muted mt-0.5 truncate">
                      {engine.label}
                      {where && <> on <span class="font-mono">{where}</span></>}
                    </p>
                    <div class="flex items-center gap-1 mt-2 -ml-0.5">
                      <code class="text-[12.5px] text-fg-muted truncate">{endpoint}</code>
                      <CopyButton text={endpoint} label="Copy URL" />
                    </div>
                  </div>
                  <div class="flex flex-wrap items-center gap-1.5">
                    <ButtonLink size="sm" variant="secondary" icon="table" href={`/schema?service=${encodeURIComponent(svc.name)}`}>Browse</ButtonLink>
                    <ButtonLink size="sm" variant="secondary" icon="terminal" href={`/api-explorer?service=${encodeURIComponent(svc.name)}`}>Try API</ButtonLink>
                    <Button size="sm" variant="ghost" icon="bolt" loading={testing === svc.name} onClick={() => test(svc.name)}>Test</Button>
                    <Button size="sm" variant="ghost" icon="pencil" onClick={() => setDrawer({ open: true, editing: svc })}>Edit</Button>
                    <RowMenu
                      items={[
                        svc.is_active
                          ? { label: 'Pause API', onClick: () => setActive(svc, false) }
                          : { label: 'Resume API', onClick: () => setActive(svc, true) },
                        { label: 'Remove', danger: true, onClick: () => remove(svc) },
                      ]}
                    />
                  </div>
                </div>
                {result && (
                  <p class={`mt-3 ml-[52px] text-sm ${result.ok ? 'text-ok' : 'text-bad'} break-words`} role="status">
                    <Icon name={result.ok ? 'check' : 'alert'} size={14} class="inline -mt-0.5 mr-1.5" />
                    {result.message}
                  </p>
                )}
              </div>
            );
          })}
        </Panel>
      )}

      <DatabaseDrawer
        open={drawer.open}
        editing={drawer.editing}
        initialEngine={drawer.engine}
        onClose={() => setDrawer({ open: false })}
        onSaved={onSaved}
      />
    </div>
  );
}

/** Small overflow menu for less common row actions. */
function RowMenu({ items }: { items: { label: string; onClick: () => void; danger?: boolean }[] }) {
  const [open, setOpen] = useState(false);
  useEffect(() => {
    if (!open) return;
    const close = () => setOpen(false);
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false);
    window.addEventListener('click', close);
    window.addEventListener('keydown', onKey);
    return () => {
      window.removeEventListener('click', close);
      window.removeEventListener('keydown', onKey);
    };
  }, [open]);
  return (
    <div class="relative">
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label="More actions"
        title="More actions"
        onClick={(e) => {
          e.stopPropagation();
          setOpen(!open);
        }}
        class="inline-flex items-center justify-center w-7 h-7 rounded-[6px] text-fg-muted hover:text-fg hover:bg-panel-2"
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
          <circle cx="5" cy="12" r="1.6" />
          <circle cx="12" cy="12" r="1.6" />
          <circle cx="19" cy="12" r="1.6" />
        </svg>
      </button>
      {open && (
        <div role="menu" class="absolute right-0 top-8 z-20 min-w-[160px] py-1 rounded-[8px] bg-panel shadow-[var(--shadow-float)] anim-pop">
          {items.map((it) => (
            <button
              key={it.label}
              type="button"
              role="menuitem"
              onClick={() => {
                setOpen(false);
                it.onClick();
              }}
              class={`w-full text-left px-3 h-8 text-sm hover:bg-panel-2 ${it.danger ? 'text-bad' : 'text-fg'}`}
            >
              {it.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
