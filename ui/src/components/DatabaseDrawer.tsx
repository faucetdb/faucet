import { useEffect, useState } from 'preact/hooks';
import { apiFetch, errorMessage } from '../hooks/useApi';
import { EngineId } from '../lib/drivers';
import {
  ConnectionForm,
  DbForm,
  FormErrors,
  ProbeResult,
  ServiceRecord,
  dbFormFromService,
  dbFormSource,
  dbFormToRequest,
  emptyDbForm,
  probeConnection,
  probeKey,
  validateDbForm,
} from './ConnectionForm';
import { FlowLine, FlowState } from './FlowLine';
import { Drawer } from './Overlay';
import { Button, Notice } from './ui';

/** Practical next steps for the errors people hit most when connecting. */
export function connectionTip(message: string, engine: EngineId): string | null {
  const m = message.toLowerCase();
  if (m.includes('connection refused') || m.includes('actively refused')) {
    return 'Nothing is listening at that host and port. Check both, and that the database accepts TCP connections. If Faucet runs in Docker, "localhost" means the container; try host.docker.internal or the host\'s IP.';
  }
  if (m.includes('no such host') || m.includes('server misbehaving') || m.includes('lookup ')) {
    return 'The host name could not be resolved. Check for typos, or use the IP address.';
  }
  if (m.includes('timed out') || m.includes('timeout') || m.includes('i/o timeout')) {
    return 'The database did not answer. A firewall or security group may be blocking Faucet, or the host is wrong.';
  }
  if (m.includes('password') || m.includes('access denied') || m.includes('login failed') || m.includes('authentication') || m.includes('ora-01017')) {
    return 'The database rejected the username or password.';
  }
  if (m.includes('does not exist') || m.includes('unknown database') || m.includes('cannot open database')) {
    return engine === 'oracle' ? 'That service name was not found. Check it with your DBA or in tnsnames.ora.' : 'That database name was not found on the server.';
  }
  if (m.includes('ssl') || m.includes('tls') || m.includes('certificate') || m.includes('x509')) {
    return 'The TLS settings do not match the server. Try a different SSL or encryption option.';
  }
  if (engine === 'sqlite' && (m.includes('unable to open') || m.includes('out of memory') || m.includes('no such file'))) {
    return 'Faucet could not open that file. The path must exist on the machine running Faucet and be readable by it.';
  }
  return null;
}

export interface DatabaseEditorOptions {
  open: boolean;
  editing?: ServiceRecord | null;
  initialEngine?: EngineId;
  onSaved: (svc: ServiceRecord, warning?: string) => void;
}

/**
 * State and actions for adding or editing a database: validation, "Test
 * connection", and test-before-save. Used by the drawer and by first-run
 * setup so both behave the same.
 */
export function useDatabaseEditor({ open, editing, initialEngine, onSaved }: DatabaseEditorOptions) {
  const [form, setForm] = useState<DbForm>(emptyDbForm());
  const [initial, setInitial] = useState<DbForm>(emptyDbForm());
  const [errors, setErrors] = useState<FormErrors>({});
  const [probe, setProbe] = useState<ProbeResult | null>(null);
  const [testing, setTesting] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [offerSaveAnyway, setOfferSaveAnyway] = useState(false);

  useEffect(() => {
    if (!open) return;
    const f = editing ? dbFormFromService(editing) : emptyDbForm(initialEngine || 'postgres');
    setForm(f);
    setInitial(f);
    setErrors({});
    setProbe(null);
    setSaveError(null);
    setOfferSaveAnyway(false);
  }, [open, editing, initialEngine]);

  const currentKey = probeKey(form);
  const freshProbe = probe && probe.key === currentKey ? probe : null;
  const flow: FlowState = testing ? 'testing' : freshProbe ? (freshProbe.ok ? 'live' : 'error') : 'idle';

  function update(f: DbForm) {
    setForm(f);
    setOfferSaveAnyway(false);
    setSaveError(null);
    // Clear field errors as they get fixed.
    if (Object.keys(errors).length) setErrors(validateDbForm(f, !!editing));
  }

  async function runTest(): Promise<ProbeResult | null> {
    const errs = validateDbForm(form, !!editing);
    // A test doesn't need an API name yet.
    delete errs.name;
    setErrors(errs);
    if (Object.keys(errs).length) return null;
    setTesting(true);
    const res = await probeConnection(form, editing?.name);
    setProbe(res);
    setTesting(false);
    return res;
  }

  /** Body for create/update. Edits only resend the connection if it changed. */
  function requestBody(): Record<string, unknown> {
    const body = dbFormToRequest(form);
    if (!editing) return body;
    const before = dbFormToRequest(initial);
    const connChanged = JSON.stringify(body.connection) !== JSON.stringify(before.connection) || !!body.dsn;
    const out: Record<string, unknown> = {
      is_active: editing.is_active,
      read_only: body.read_only,
      raw_sql_allowed: body.raw_sql_allowed,
    };
    // Send schema whenever it changed, including clearing it.
    if (form.schema.trim() !== initial.schema.trim()) out.schema = form.schema.trim();
    if (body.private_key_path) out.private_key_path = body.private_key_path;
    // Switching Snowflake from key pair to password clears the stored key.
    if (initial.authMethod === 'keypair' && form.authMethod === 'password') out.private_key_path = '';
    if (connChanged) {
      if (body.dsn) out.dsn = body.dsn;
      else out.connection = body.connection;
    }
    return out;
  }

  async function save(force = false) {
    const errs = validateDbForm(form, !!editing);
    setErrors(errs);
    if (Object.keys(errs).length) return;
    setSaveError(null);

    // Test first unless the current settings already passed, so nobody saves
    // a database that can't connect without knowing it.
    if (!force && !(freshProbe && freshProbe.ok)) {
      const res = await runTest();
      if (!res || !res.ok) {
        setOfferSaveAnyway(true);
        return;
      }
    }

    setSaving(true);
    try {
      const body = requestBody();
      const svc: any = editing
        ? await apiFetch(`/api/v1/system/service/${encodeURIComponent(editing.name)}`, { method: 'PUT', body })
        : await apiFetch('/api/v1/system/service', { method: 'POST', body });
      onSaved(svc as ServiceRecord, svc?.connection_warning);
    } catch (err) {
      setSaveError(errorMessage(err, 'The database could not be saved.'));
    } finally {
      setSaving(false);
    }
  }

  return { form, update, errors, flow, freshProbe, testing, saving, saveError, offerSaveAnyway, runTest, save, editing: !!editing };
}

export type DatabaseEditor = ReturnType<typeof useDatabaseEditor>;

/** Flow line, test result and the connection form. */
export function DatabaseEditorBody({ editor }: { editor: DatabaseEditor }) {
  const { form, freshProbe, offerSaveAnyway, saveError } = editor;
  const tip = freshProbe && !freshProbe.ok ? connectionTip(freshProbe.message, form.engine) : null;
  return (
    <div class="flex flex-col gap-6">
      <FlowLine engine={form.engine} source={dbFormSource(form)} apiName={form.name} state={editor.flow} />

      {freshProbe &&
        (freshProbe.ok ? (
          <Notice tone="ok" title={freshProbe.message}>
            {!!freshProbe.latencyMs && <span>Responded in {freshProbe.latencyMs} ms.</span>}
          </Notice>
        ) : (
          <Notice tone="bad" title={offerSaveAnyway ? 'Faucet could not connect, so nothing was saved yet' : 'Faucet could not connect'}>
            {tip && <p class="text-fg mb-1">{tip}</p>}
            <p class="font-mono text-xs break-all">{freshProbe.message}</p>
            {offerSaveAnyway && <p class="mt-1">Fix the settings and test again, or save anyway if the database is not reachable yet.</p>}
          </Notice>
        ))}
      {saveError && <Notice tone="bad" title="Not saved">{saveError}</Notice>}

      <ConnectionForm form={form} onChange={editor.update} errors={editor.errors} editing={editor.editing} />
    </div>
  );
}

/** Test and save buttons; the save button turns into "Save anyway" after a failed test. */
export function DatabaseEditorActions({ editor, saveLabel }: { editor: DatabaseEditor; saveLabel: string }) {
  return (
    <>
      <Button variant="secondary" icon="bolt" onClick={editor.runTest} loading={editor.testing} disabled={editor.saving}>
        Test connection
      </Button>
      {editor.offerSaveAnyway ? (
        <Button variant="danger" onClick={() => editor.save(true)} loading={editor.saving}>
          Save anyway
        </Button>
      ) : (
        <Button variant="primary" onClick={() => editor.save()} loading={editor.saving} disabled={editor.testing}>
          {saveLabel}
        </Button>
      )}
    </>
  );
}

/** Add or edit a database in a drawer. */
export function DatabaseDrawer({ open, onClose, editing, initialEngine, onSaved }: DatabaseEditorOptions & { onClose: () => void }) {
  const editor = useDatabaseEditor({ open, editing, initialEngine, onSaved });
  return (
    <Drawer
      open={open}
      onClose={onClose}
      title={editing ? `Edit ${editing.name}` : 'Add a database'}
      description={editing ? 'Change how Faucet connects to this database.' : 'Faucet turns it into a REST API and MCP tools as soon as you save.'}
      width="max-w-[640px]"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} class="hidden sm:inline-flex">Cancel</Button>
          <span class="hidden sm:block flex-1" />
          <DatabaseEditorActions editor={editor} saveLabel={editing ? 'Save changes' : 'Add database'} />
        </>
      }
    >
      <DatabaseEditorBody editor={editor} />
    </Drawer>
  );
}
