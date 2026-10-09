import { useState } from 'preact/hooks';
import { ServiceRecord } from '../components/ConnectionForm';
import { DatabaseEditorActions, DatabaseEditorBody, useDatabaseEditor } from '../components/DatabaseDrawer';
import { Icon } from '../components/Icon';
import { Button, Field, Input, Notice, PasswordInput, useId } from '../components/ui';
import { errorMessage } from '../hooks/useApi';

type Step = 'admin' | 'database' | 'done';

const STEPS: { key: Step; label: string }[] = [
  { key: 'admin', label: 'Create your admin account' },
  { key: 'database', label: 'Connect a database' },
  { key: 'done', label: 'Make your first request' },
];

/**
 * First-run setup. Shown when the server has no admin account yet.
 * `onComplete` receives the page to open next.
 */
export function Setup({ onComplete }: { onComplete: (next?: string) => void }) {
  const [step, setStep] = useState<Step>('admin');
  const [saved, setSaved] = useState<ServiceRecord | null>(null);
  const currentIdx = STEPS.findIndex((s) => s.key === step);

  return (
    <div class="min-h-screen px-4 py-10 sm:py-14">
      <div class="max-w-[640px] mx-auto">
        <div class="flex items-center gap-2.5 mb-8">
          <img src="/faucet-icon.svg" alt="" width="28" height="28" />
          <span class="text-lg font-semibold tracking-[-0.01em] text-fg">Faucet</span>
        </div>

        <ol class="flex flex-col sm:flex-row gap-2 sm:gap-6 mb-8" aria-label="Setup progress">
          {STEPS.map((s, i) => {
            const done = i < currentIdx;
            const current = i === currentIdx;
            return (
              <li key={s.key} class="flex items-center gap-2" aria-current={current ? 'step' : undefined}>
                <span
                  class={`inline-flex items-center justify-center w-6 h-6 rounded-full text-xs font-semibold ${
                    done ? 'bg-brand text-white' : current ? 'border-2 border-brand text-brand-fg' : 'border border-line-strong text-fg-faint'
                  }`}
                >
                  {done ? <Icon name="check" size={13} /> : i + 1}
                </span>
                <span class={`text-sm ${current ? 'text-fg font-medium' : 'text-fg-muted'}`}>{s.label}</span>
              </li>
            );
          })}
        </ol>

        {step === 'admin' && <AdminStep onDone={() => setStep('database')} />}
        {step === 'database' && (
          <DatabaseStep
            onSaved={(svc) => {
              setSaved(svc);
              setStep('done');
            }}
            onSkip={() => setStep('done')}
          />
        )}
        {step === 'done' && <DoneStep service={saved} onComplete={onComplete} />}
      </div>
    </div>
  );
}

function AdminStep({ onDone }: { onDone: () => void }) {
  const [form, setForm] = useState({ email: '', password: '', confirm: '' });
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const ids = { email: useId('email'), pass: useId('pass'), confirm: useId('confirm') };
  const mismatch = form.confirm.length > 0 && form.confirm !== form.password;
  const short = form.password.length > 0 && form.password.length < 8;

  async function submit(e: Event) {
    e.preventDefault();
    if (form.password !== form.confirm) return setError('The passwords don\'t match.');
    if (form.password.length < 8) return setError('Use at least 8 characters for the password.');
    setSaving(true);
    setError(null);
    try {
      const res = await fetch('/api/v1/setup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: form.email, password: form.password }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error?.message || `Setup failed (${res.status})`);
      if (data.session_token) localStorage.setItem('faucet_session', data.session_token);
      localStorage.setItem('faucet_admin_email', form.email);
      onDone();
    } catch (err) {
      setError(errorMessage(err, 'Could not create the admin account'));
    } finally {
      setSaving(false);
    }
  }

  return (
    <form onSubmit={submit} noValidate>
      <h1 class="text-2xl font-semibold tracking-[-0.015em] text-fg">Welcome to Faucet</h1>
      <p class="text-base text-fg-muted mt-2 mb-7 max-w-[56ch]">
        Faucet turns your database into a REST API and an MCP server for AI agents. First, create the admin account you'll use to sign in here.
      </p>
      <div class="flex flex-col gap-4 max-w-[400px]">
        {error && <Notice tone="bad" title={error} />}
        <Field label="Email" htmlFor={ids.email}>
          <Input id={ids.email} type="email" value={form.email} onValue={(email) => setForm({ ...form, email })} autocomplete="username" autoFocus placeholder="you@example.com" />
        </Field>
        <Field label="Password" htmlFor={ids.pass} hint="At least 8 characters." error={short ? 'At least 8 characters.' : null}>
          <PasswordInput id={ids.pass} value={form.password} onValue={(password) => setForm({ ...form, password })} autocomplete="new-password" invalid={short} />
        </Field>
        <Field label="Confirm password" htmlFor={ids.confirm} error={mismatch ? 'Doesn\'t match the password above.' : null}>
          <PasswordInput id={ids.confirm} value={form.confirm} onValue={(confirm) => setForm({ ...form, confirm })} autocomplete="new-password" invalid={mismatch} />
        </Field>
        <Button type="submit" variant="primary" loading={saving} disabled={!form.email || !form.password || !form.confirm} class="mt-2 self-start">
          Create account
        </Button>
      </div>
    </form>
  );
}

function DatabaseStep({ onSaved, onSkip }: { onSaved: (svc: ServiceRecord) => void; onSkip: () => void }) {
  const editor = useDatabaseEditor({ open: true, onSaved: (svc) => onSaved(svc) });
  return (
    <div>
      <h1 class="text-2xl font-semibold tracking-[-0.015em] text-fg">Connect a database</h1>
      <p class="text-base text-fg-muted mt-2 mb-7 max-w-[56ch]">
        Enter the details you'd give any database client. Faucet tests the connection before saving, and you can add more databases later.
      </p>
      <DatabaseEditorBody editor={editor} />
      <div class="flex flex-wrap items-center gap-2 mt-8 pt-5 border-t border-line">
        <Button variant="ghost" onClick={onSkip}>Skip for now</Button>
        <span class="flex-1" />
        <DatabaseEditorActions editor={editor} saveLabel="Add database" />
      </div>
    </div>
  );
}

function DoneStep({ service, onComplete }: { service: ServiceRecord | null; onComplete: (next?: string) => void }) {
  if (!service) {
    return (
      <div>
        <h1 class="text-2xl font-semibold tracking-[-0.015em] text-fg">Your account is ready</h1>
        <p class="text-base text-fg-muted mt-2 mb-7 max-w-[56ch]">
          Connect a database whenever you're ready. Faucet gives it a REST API and MCP tools the moment you save it.
        </p>
        <div class="flex flex-wrap gap-2">
          <Button variant="primary" icon="plus" onClick={() => onComplete('/services?add=1')}>Add a database</Button>
          <Button variant="ghost" onClick={() => onComplete('/')}>Go to overview</Button>
        </div>
      </div>
    );
  }
  return (
    <div>
      <h1 class="text-2xl font-semibold tracking-[-0.015em] text-fg">
        <span class="font-mono">{service.name}</span> is live
      </h1>
      <p class="text-base text-fg-muted mt-2 mb-7 max-w-[56ch]">
        Every table now has REST endpoints at <span class="font-mono text-fg">/api/v1/{service.name}/_table/…</span>, and AI agents can reach it through MCP.
        Pick where to go next.
      </p>
      <div class="grid sm:grid-cols-3 gap-3">
        <NextCard icon="table" title="Browse your tables" body="See columns, keys and sample rows." onClick={() => onComplete(`/schema?service=${encodeURIComponent(service.name)}`)} />
        <NextCard icon="key" title="Create an API key" body="Give an app or teammate scoped access." onClick={() => onComplete('/api-keys')} />
        <NextCard icon="sparkle" title="Connect an AI agent" body="Claude, Cursor, ChatGPT and more." onClick={() => onComplete('/mcp')} />
      </div>
      <Button variant="ghost" class="mt-6 -ml-3" onClick={() => onComplete('/')}>Go to overview</Button>
    </div>
  );
}

function NextCard({ icon, title, body, onClick }: { icon: 'table' | 'key' | 'sparkle'; title: string; body: string; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} class="text-left p-4 rounded-[10px] border border-line bg-panel hover:border-line-strong hover:bg-panel-2 transition-colors">
      <Icon name={icon} size={18} class="text-brand-fg" />
      <p class="text-base font-medium text-fg mt-3">{title}</p>
      <p class="text-sm text-fg-muted mt-0.5">{body}</p>
    </button>
  );
}
