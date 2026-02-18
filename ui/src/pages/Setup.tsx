import { useState } from 'preact/hooks';
import { route } from 'preact-router';
import { apiFetch } from '../hooks/useApi';

type Step = 'welcome' | 'admin' | 'database' | 'done';

const DB_TYPES = [
  { value: 'postgres', label: 'PostgreSQL', port: 5432 },
  { value: 'mysql', label: 'MySQL', port: 3306 },
  { value: 'mssql', label: 'SQL Server', port: 1433 },
  { value: 'snowflake', label: 'Snowflake', port: 443 },
];

export function Setup() {
  const [step, setStep] = useState<Step>('welcome');
  const [adminForm, setAdminForm] = useState({
    username: 'admin',
    password: '',
    confirmPassword: '',
  });
  const [dbForm, setDbForm] = useState({
    name: '',
    type: 'postgres',
    host: 'localhost',
    port: 5432,
    database: '',
    username: '',
    password: '',
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
      await apiFetch('/api/v1/system/setup/admin', {
        method: 'POST',
        body: {
          username: adminForm.username,
          password: adminForm.password,
        },
      });
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
      await apiFetch('/api/v1/services', {
        method: 'POST',
        body: dbForm,
      });
      setStep('done');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add database');
    } finally {
      setSaving(false);
    }
  }

  function handleTypeChange(type: string) {
    const dbType = DB_TYPES.find((t) => t.value === type);
    setDbForm({
      ...dbForm,
      type,
      port: dbType?.port || dbForm.port,
    });
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
              <div class="w-16 h-16 rounded-2xl bg-gradient-to-br from-brand to-cyan-accent flex items-center justify-center mx-auto mb-6">
                <svg class="w-8 h-8 text-white" viewBox="0 0 20 20" fill="currentColor">
                  <path d="M5 3a2 2 0 00-2 2v2a2 2 0 002 2h2a2 2 0 002-2V5a2 2 0 00-2-2H5zM5 11a2 2 0 00-2 2v2a2 2 0 002 2h2a2 2 0 002-2v-2a2 2 0 00-2-2H5zM11 5a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V5zM14 11a1 1 0 011 1v1h1a1 1 0 110 2h-1v1a1 1 0 11-2 0v-1h-1a1 1 0 110-2h1v-1a1 1 0 011-1z" />
                </svg>
              </div>
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
                  <label class="block text-sm font-medium text-text-secondary mb-1.5">Username</label>
                  <input
                    type="text"
                    class="input w-full"
                    value={adminForm.username}
                    onInput={(e) => setAdminForm({ ...adminForm, username: (e.target as HTMLInputElement).value })}
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
                  disabled={saving || !adminForm.password}
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
                  <label class="block text-sm font-medium text-text-secondary mb-1.5">Database Type</label>
                  <div class="grid grid-cols-2 gap-2">
                    {DB_TYPES.map((t) => (
                      <button
                        key={t.value}
                        onClick={() => handleTypeChange(t.value)}
                        class={`
                          p-3 rounded-lg border text-left text-sm font-medium transition-colors
                          ${dbForm.type === t.value
                            ? 'border-brand bg-brand/10 text-brand'
                            : 'border-border-default bg-surface hover:bg-surface-overlay text-text-secondary'
                          }
                        `}
                      >
                        {t.label}
                      </button>
                    ))}
                  </div>
                </div>

                <div class="grid grid-cols-3 gap-3">
                  <div class="col-span-2">
                    <label class="block text-sm font-medium text-text-secondary mb-1.5">Host</label>
                    <input
                      type="text"
                      class="input w-full font-mono text-sm"
                      value={dbForm.host}
                      onInput={(e) => setDbForm({ ...dbForm, host: (e.target as HTMLInputElement).value })}
                    />
                  </div>
                  <div>
                    <label class="block text-sm font-medium text-text-secondary mb-1.5">Port</label>
                    <input
                      type="number"
                      class="input w-full font-mono text-sm"
                      value={dbForm.port}
                      onInput={(e) => setDbForm({ ...dbForm, port: parseInt((e.target as HTMLInputElement).value) || 0 })}
                    />
                  </div>
                </div>

                <div>
                  <label class="block text-sm font-medium text-text-secondary mb-1.5">Database Name</label>
                  <input
                    type="text"
                    class="input w-full font-mono text-sm"
                    placeholder="mydb"
                    value={dbForm.database}
                    onInput={(e) => setDbForm({ ...dbForm, database: (e.target as HTMLInputElement).value })}
                  />
                </div>

                <div class="grid grid-cols-2 gap-3">
                  <div>
                    <label class="block text-sm font-medium text-text-secondary mb-1.5">Username</label>
                    <input
                      type="text"
                      class="input w-full font-mono text-sm"
                      value={dbForm.username}
                      onInput={(e) => setDbForm({ ...dbForm, username: (e.target as HTMLInputElement).value })}
                    />
                  </div>
                  <div>
                    <label class="block text-sm font-medium text-text-secondary mb-1.5">Password</label>
                    <input
                      type="password"
                      class="input w-full font-mono text-sm"
                      value={dbForm.password}
                      onInput={(e) => setDbForm({ ...dbForm, password: (e.target as HTMLInputElement).value })}
                    />
                  </div>
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
                  disabled={saving || !dbForm.name || !dbForm.database}
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
                  onClick={() => route('/', true)}
                  class="btn-primary text-base px-6 py-2.5"
                >
                  Go to Dashboard
                </button>
                {!skipDb && (
                  <a href="/schema" class="btn-secondary text-base px-6 py-2.5">
                    Explore Schema
                  </a>
                )}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
