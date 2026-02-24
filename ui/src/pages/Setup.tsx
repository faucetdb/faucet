import { useState } from 'preact/hooks';
import { route } from 'preact-router';
import { apiFetch } from '../hooks/useApi';

type Step = 'welcome' | 'admin' | 'database' | 'done';

const DB_DRIVERS = [
  { value: 'postgres', label: 'PostgreSQL' },
  { value: 'mysql', label: 'MySQL' },
  { value: 'mssql', label: 'SQL Server' },
  { value: 'snowflake', label: 'Snowflake' },
];

interface SetupProps {
  onComplete?: () => void;
}

export function Setup({ onComplete }: SetupProps = {}) {
  const [step, setStep] = useState<Step>('welcome');
  const [adminForm, setAdminForm] = useState({
    email: '',
    password: '',
    confirmPassword: '',
  });
  const [dbForm, setDbForm] = useState({
    name: '',
    driver: 'postgres',
    dsn: '',
    schema: '',
  });
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [skipDb, setSkipDb] = useState(false);

  async function handleAdminSubmit() {
    if (adminForm.password !== adminForm.confirmPassword) {
      setError('Passwords do not match');
      return;
    }
    if (adminForm.password.length < 8) {
      setError('Password must be at least 8 characters');
      return;
    }
    setSaving(true);
    setError(null);
    try {
      const res = await fetch('/api/v1/setup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          email: adminForm.email,
          password: adminForm.password,
        }),
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body.error?.message || `Setup failed (${res.status})`);
      }
      const data = await res.json();
      // Store the session token so subsequent API calls are authenticated
      if (data.session_token) {
        localStorage.setItem('faucet_session', data.session_token);
      }
      setStep('database');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create admin account');
    } finally {
      setSaving(false);
    }
  }

  async function handleDbSubmit() {
    setSaving(true);
    setError(null);
    try {
      const body: Record<string, any> = {
        name: dbForm.name,
        driver: dbForm.driver,
        dsn: dbForm.dsn,
      };
      if (dbForm.schema) {
        body.schema = dbForm.schema;
      }
      await apiFetch('/api/v1/system/service', {
        method: 'POST',
        body,
      });
      setStep('done');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add database');
    } finally {
      setSaving(false);
    }
  }

  function dsnPlaceholder(driver: string): string {
    switch (driver) {
      case 'postgres':
        return 'postgres://user:pass@localhost:5432/dbname?sslmode=disable';
      case 'mysql':
        return 'user:pass@tcp(localhost:3306)/dbname';
      case 'mssql':
        return 'sqlserver://user:pass@localhost:1433?database=dbname';
      case 'snowflake':
        return 'user:pass@account/dbname/schema?warehouse=wh';
      default:
        return '';
    }
  }

  const steps: { key: Step; label: string }[] = [
    { key: 'welcome', label: 'Welcome' },
    { key: 'admin', label: 'Admin' },
    { key: 'database', label: 'Database' },
    { key: 'done', label: 'Done' },
  ];

  const stepOrder: Step[] = ['welcome', 'admin', 'database', 'done'];
  const currentIdx = stepOrder.indexOf(step);

  return (
    <div class="min-h-[80vh] flex items-center justify-center">
      <div class="w-full max-w-xl">
        {/* Progress */}
        <div class="flex items-center justify-center gap-2 mb-8">
          {steps.map((s, idx) => (
            <div key={s.key} class="flex items-center gap-2">
              <div
                class={`
                  w-8 h-8 rounded-full flex items-center justify-center text-xs font-semibold
                  transition-colors
                  ${idx < currentIdx
                    ? 'bg-brand text-white'
                    : idx === currentIdx
                    ? 'bg-brand/20 text-brand border-2 border-brand'
                    : 'bg-surface-overlay text-text-muted border border-border-default'
                  }
                `}
              >
                {idx < currentIdx ? (
                  <svg class="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
                    <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                  </svg>
                ) : (
                  idx + 1
                )}
              </div>
              {idx < steps.length - 1 && (
                <div class={`w-12 h-0.5 ${idx < currentIdx ? 'bg-brand' : 'bg-border-default'}`} />
              )}
            </div>
          ))}
        </div>

        {/* Step content */}
        <div class="card animate-fade-in">
          {step === 'welcome' && (
            <div class="text-center py-8">
              <img src="/faucet-icon.svg" alt="Faucet" width="64" height="64" class="mx-auto mb-6" />
              <h1 class="text-2xl font-bold text-text-primary mb-3">Welcome to Faucet</h1>
              <p class="text-text-secondary mb-8 max-w-sm mx-auto">
                Turn any SQL database into a secure REST API in minutes.
                Let's get your instance configured.
              </p>
              <button
                onClick={() => setStep('admin')}
                class="btn-primary text-base px-8 py-3"
              >
                Get Started
              </button>
            </div>
          )}

          {step === 'admin' && (
            <div>
              <h2 class="text-xl font-semibold text-text-primary mb-2">Create Admin Account</h2>
              <p class="text-sm text-text-secondary mb-6">
                Set up the administrator account for the Faucet dashboard.
              </p>

              {error && (
                <div class="p-3 rounded-lg bg-error/10 border border-error/20 text-sm text-error mb-4">
                  {error}
                </div>
              )}

              <div class="space-y-4">
                <div>
                  <label class="block text-sm font-medium text-text-secondary mb-1.5">Email</label>
                  <input
                    type="email"
                    class="input w-full"
                    placeholder="admin@example.com"
                    value={adminForm.email}
                    autoFocus
                    onInput={(e) => setAdminForm({ ...adminForm, email: (e.target as HTMLInputElement).value })}
                  />
                </div>
                <div>
                  <label class="block text-sm font-medium text-text-secondary mb-1.5">Password</label>
                  <input
                    type="password"
                    class="input w-full"
                    placeholder="Minimum 8 characters"
                    value={adminForm.password}
                    onInput={(e) => setAdminForm({ ...adminForm, password: (e.target as HTMLInputElement).value })}
                  />
                </div>
                <div>
                  <label class="block text-sm font-medium text-text-secondary mb-1.5">Confirm Password</label>
                  <input
                    type="password"
                    class="input w-full"
                    placeholder="Retype password"
                    value={adminForm.confirmPassword}
                    onInput={(e) => setAdminForm({ ...adminForm, confirmPassword: (e.target as HTMLInputElement).value })}
                  />
                </div>
              </div>

              <div class="flex justify-end mt-6 pt-4 border-t border-border-subtle">
                <button
                  onClick={handleAdminSubmit}
                  disabled={saving || !adminForm.email || !adminForm.password}
                  class="btn-primary"
                >
                  {saving ? 'Creating...' : 'Create Admin & Continue'}
                </button>
              </div>
            </div>
          )}

          {step === 'database' && (
            <div>
              <h2 class="text-xl font-semibold text-text-primary mb-2">Connect a Database</h2>
              <p class="text-sm text-text-secondary mb-6">
                Add your first database connection. You can add more later.
              </p>

              {error && (
                <div class="p-3 rounded-lg bg-error/10 border border-error/20 text-sm text-error mb-4">
                  {error}
                </div>
              )}

              <div class="space-y-4">
                <div>
                  <label class="block text-sm font-medium text-text-secondary mb-1.5">Service Name</label>
                  <input
                    type="text"
                    class="input w-full"
                    placeholder="my-database"
                    value={dbForm.name}
                    onInput={(e) => setDbForm({ ...dbForm, name: (e.target as HTMLInputElement).value })}
                  />
                </div>

                <div>
                  <label class="block text-sm font-medium text-text-secondary mb-1.5">Database Driver</label>
                  <div class="grid grid-cols-2 gap-2">
                    {DB_DRIVERS.map((d) => (
                      <button
                        key={d.value}
                        onClick={() => setDbForm({ ...dbForm, driver: d.value })}
                        class={`
                          p-3 rounded-lg border text-left text-sm font-medium transition-colors
                          ${dbForm.driver === d.value
                            ? 'border-brand bg-brand/10 text-brand'
                            : 'border-border-default bg-surface hover:bg-surface-overlay text-text-secondary'
                          }
                        `}
                      >
                        {d.label}
                      </button>
                    ))}
                  </div>
                </div>

                <div>
                  <label class="block text-sm font-medium text-text-secondary mb-1.5">DSN (Connection String)</label>
                  <input
                    type="text"
                    class="input w-full font-mono text-sm"
                    placeholder={dsnPlaceholder(dbForm.driver)}
                    value={dbForm.dsn}
                    onInput={(e) => setDbForm({ ...dbForm, dsn: (e.target as HTMLInputElement).value })}
                  />
                </div>

                <div>
                  <label class="block text-sm font-medium text-text-secondary mb-1.5">
                    Schema <span class="text-text-muted font-normal">(optional)</span>
                  </label>
                  <input
                    type="text"
                    class="input w-full font-mono text-sm"
                    placeholder="public"
                    value={dbForm.schema}
                    onInput={(e) => setDbForm({ ...dbForm, schema: (e.target as HTMLInputElement).value })}
                  />
                </div>
              </div>

              <div class="flex items-center justify-between mt-6 pt-4 border-t border-border-subtle">
                <button
                  onClick={() => { setSkipDb(true); setStep('done'); }}
                  class="btn-ghost text-sm"
                >
                  Skip for now
                </button>
                <button
                  onClick={handleDbSubmit}
                  disabled={saving || !dbForm.name || !dbForm.dsn}
                  class="btn-primary"
                >
                  {saving ? 'Connecting...' : 'Connect & Finish'}
                </button>
              </div>
            </div>
          )}

          {step === 'done' && (
            <div class="text-center py-8">
              <div class="w-16 h-16 rounded-full bg-success/10 flex items-center justify-center mx-auto mb-6">
                <svg class="w-8 h-8 text-success" viewBox="0 0 20 20" fill="currentColor">
                  <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                </svg>
              </div>
              <h2 class="text-2xl font-bold text-text-primary mb-3">You're All Set!</h2>
              <p class="text-text-secondary mb-8 max-w-sm mx-auto">
                {skipDb
                  ? 'Your Faucet instance is ready. Add a database connection to start building APIs.'
                  : 'Your database is connected and REST APIs are live. Explore your schema or test the API.'
                }
              </p>
              <div class="flex items-center justify-center gap-3">
                <button
                  onClick={() => {
                    if (onComplete) {
                      onComplete();
                    } else {
                      window.location.href = '/';
                    }
                  }}
                  class="btn-primary text-base px-6 py-2.5"
                >
                  Go to Dashboard
                </button>
                {!skipDb && (
                  <button
                    onClick={() => {
                      if (onComplete) {
                        onComplete();
                      }
                      route('/schema', true);
                    }}
                    class="btn-secondary text-base px-6 py-2.5"
                  >
                    Explore Schema
                  </button>
                )}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
