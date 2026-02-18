import { useState, useEffect } from 'preact/hooks';
import { Modal } from '../components/Modal';
import { StatusBadge } from '../components/StatusBadge';
import { apiFetch } from '../hooks/useApi';

interface Service {
  name: string;
  driver: string;
  dsn: string;
  schema: string;
  is_active: boolean;
  created_at: string;
}

const DB_DRIVERS = [
  { value: 'postgres', label: 'PostgreSQL' },
  { value: 'mysql', label: 'MySQL' },
  { value: 'mssql', label: 'SQL Server' },
  { value: 'snowflake', label: 'Snowflake' },
];

const emptyForm = {
  name: '',
  driver: 'postgres',
  dsn: '',
  schema: '',
};

export function Services() {
  const [services, setServices] = useState<Service[]>([]);
  const [loading, setLoading] = useState(true);
  const [showModal, setShowModal] = useState(false);
  const [form, setForm] = useState({ ...emptyForm });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [testing, setTesting] = useState<string | null>(null);
  const [testResults, setTestResults] = useState<Record<string, { ok: boolean; message: string }>>({});

  useEffect(() => {
    loadServices();
  }, []);

  async function loadServices() {
    setLoading(true);
    try {
      const res = await apiFetch('/api/v1/system/service');
      setServices(res.resource || []);
    } catch {
      setServices([]);
    } finally {
      setLoading(false);
    }
  }

  function openNew() {
    setForm({ ...emptyForm });
    setError(null);
    setShowModal(true);
  }

  async function handleTestConnection(serviceName: string) {
    setTesting(serviceName);
    try {
      await apiFetch(`/api/v1/system/service/${serviceName}/test`);
      setTestResults({ ...testResults, [serviceName]: { ok: true, message: 'Connection successful' } });
    } catch (err) {
      setTestResults({
        ...testResults,
        [serviceName]: {
          ok: false,
          message: err instanceof Error ? err.message : 'Connection failed',
        },
      });
    } finally {
      setTesting(null);
    }
  }

  async function handleSave() {
    setSaving(true);
    setError(null);
    try {
      const body: Record<string, any> = {
        name: form.name,
        driver: form.driver,
        dsn: form.dsn,
      };
      if (form.schema) {
        body.schema = form.schema;
      }

      const result = await apiFetch('/api/v1/system/service', { method: 'POST', body });
      setShowModal(false);
      await loadServices();

      // If the backend returned a connection warning, show it on the service card.
      if (result?.connection_warning) {
        setTestResults({
          ...testResults,
          [form.name]: { ok: false, message: result.connection_warning },
        });
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save service');
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete(name: string) {
    if (!confirm(`Delete service "${name}"? This cannot be undone.`)) return;
    try {
      await apiFetch(`/api/v1/system/service/${name}`, { method: 'DELETE' });
      loadServices();
    } catch {
      // ignore
    }
  }

  function dsnPlaceholder(driver: string): string {
    switch (driver) {
      case 'postgres':
        return 'postgres://user:pass@localhost:5432/dbname?sslmode=disable';
      case 'mysql':
        return 'user:pass@tcp(host:3306)/dbname';
      case 'mssql':
        return 'sqlserver://user:pass@localhost:1433?database=dbname';
      case 'snowflake':
        return 'user:pass@account/dbname/schema?warehouse=wh';
      default:
        return '';
    }
  }

  function dsnHelpText(driver: string): string {
    switch (driver) {
      case 'mysql':
        return 'Format: user:pass@tcp(host:port)/dbname — the tcp() wrapper is required';
      case 'postgres':
        return 'Format: postgres://user:pass@host:port/dbname?sslmode=disable';
      case 'mssql':
        return 'Format: sqlserver://user:pass@host:port?database=dbname';
      default:
        return 'Full connection string for the database';
    }
  }

  return (
    <div class="space-y-6">
      {/* Page header */}
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-text-primary">Services</h1>
          <p class="text-sm text-text-secondary mt-1">Manage database connections</p>
        </div>
        <button onClick={openNew} class="btn-primary flex items-center gap-2">
          <svg class="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M10 3a1 1 0 011 1v5h5a1 1 0 110 2h-5v5a1 1 0 11-2 0v-5H4a1 1 0 110-2h5V4a1 1 0 011-1z" clip-rule="evenodd" />
          </svg>
          Add Service
        </button>
      </div>

      {/* Service cards */}
      {loading ? (
        <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {[1, 2, 3].map((i) => (
            <div key={i} class="card animate-pulse">
              <div class="h-4 bg-surface-overlay rounded w-1/2 mb-4" />
              <div class="h-3 bg-surface-overlay rounded w-3/4 mb-2" />
              <div class="h-3 bg-surface-overlay rounded w-1/3" />
            </div>
          ))}
        </div>
      ) : services.length === 0 ? (
        <div class="card text-center py-16">
          <div class="text-text-muted mb-4">
            <svg class="w-12 h-12 mx-auto" viewBox="0 0 20 20" fill="currentColor" opacity="0.3">
              <path d="M3 12v3c0 1.657 3.134 3 7 3s7-1.343 7-3v-3c0 1.657-3.134 3-7 3s-7-1.343-7-3z" />
              <path d="M3 7v3c0 1.657 3.134 3 7 3s7-1.343 7-3V7c0 1.657-3.134 3-7 3S3 8.657 3 7z" />
              <path d="M17 5c0 1.657-3.134 3-7 3S3 6.657 3 5s3.134-3 7-3 7 1.343 7 3z" />
            </svg>
          </div>
          <h3 class="text-lg font-medium text-text-primary mb-2">No services connected</h3>
          <p class="text-sm text-text-secondary mb-6">Add a database connection to start generating REST APIs.</p>
          <button onClick={openNew} class="btn-primary">Add Your First Service</button>
        </div>
      ) : (
        <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {services.map((svc) => (
            <div key={svc.name} class="card group hover:border-border-default transition-colors">
              <div class="flex items-start justify-between mb-3">
                <div class="flex items-center gap-3">
                  <div class="w-10 h-10 rounded-lg bg-brand/10 flex items-center justify-center shrink-0">
                    <span class="text-sm font-bold text-brand uppercase">{svc.driver.slice(0, 2)}</span>
                  </div>
                  <div>
                    <h3 class="text-sm font-semibold text-text-primary">{svc.name}</h3>
                    <p class="text-xs text-text-muted">{svc.driver}</p>
                  </div>
                </div>
                <StatusBadge status={svc.is_active ? 'active' : 'inactive'} />
              </div>

              {svc.schema && (
                <div class="text-xs font-mono text-text-secondary mb-3">
                  <span class="text-text-muted">schema:</span> {svc.schema}
                </div>
              )}

              {/* Test result */}
              {testResults[svc.name] && (
                <div
                  class={`text-xs p-2 rounded mb-3 ${
                    testResults[svc.name].ok
                      ? 'bg-success/10 text-success'
                      : 'bg-error/10 text-error'
                  }`}
                >
                  {testResults[svc.name].message}
                </div>
              )}

              <div class="flex items-center gap-2 pt-3 border-t border-border-subtle">
                <button
                  onClick={() => handleTestConnection(svc.name)}
                  disabled={testing === svc.name}
                  class="btn-ghost text-xs py-1.5 px-3"
                >
                  {testing === svc.name ? 'Testing...' : 'Test'}
                </button>
                <a
                  href={`/schema?service=${svc.name}`}
                  class="btn-ghost text-xs py-1.5 px-3"
                >
                  Schema
                </a>
                <button
                  onClick={() => handleDelete(svc.name)}
                  class="btn-ghost text-xs py-1.5 px-3 text-error hover:text-error ml-auto"
                >
                  Delete
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Add Modal */}
      <Modal
        isOpen={showModal}
        onClose={() => setShowModal(false)}
        title="Add Service"
        width="max-w-xl"
      >
        <div class="space-y-4">
          {error && (
            <div class="p-3 rounded-lg bg-error/10 border border-error/20 text-sm text-error">
              {error}
            </div>
          )}

          <div>
            <label class="block text-sm font-medium text-text-secondary mb-1.5">Service Name</label>
            <input
              type="text"
              class="input w-full"
              placeholder="my-database"
              value={form.name}
              onInput={(e) => setForm({ ...form, name: (e.target as HTMLInputElement).value })}
            />
            <p class="text-xs text-text-muted mt-1">Unique identifier used in API URLs (e.g. /api/v1/my-database/_table/...)</p>
          </div>

          <div>
            <label class="block text-sm font-medium text-text-secondary mb-1.5">Database Driver</label>
            <div class="grid grid-cols-2 gap-2">
              {DB_DRIVERS.map((d) => (
                <button
                  key={d.value}
                  onClick={() => setForm({ ...form, driver: d.value })}
                  class={`
                    p-3 rounded-lg border text-left text-sm font-medium transition-colors
                    ${form.driver === d.value
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
              placeholder={dsnPlaceholder(form.driver)}
              value={form.dsn}
              onInput={(e) => setForm({ ...form, dsn: (e.target as HTMLInputElement).value })}
            />
            <p class="text-xs text-text-muted mt-1">{dsnHelpText(form.driver)}</p>
          </div>

          <div>
            <label class="block text-sm font-medium text-text-secondary mb-1.5">
              Schema <span class="text-text-muted font-normal">(optional)</span>
            </label>
            <input
              type="text"
              class="input w-full font-mono text-sm"
              placeholder="public"
              value={form.schema}
              onInput={(e) => setForm({ ...form, schema: (e.target as HTMLInputElement).value })}
            />
            <p class="text-xs text-text-muted mt-1">Database schema to use (defaults to driver default)</p>
          </div>

          {/* Actions */}
          <div class="flex items-center justify-end gap-2 pt-4 border-t border-border-subtle">
            <button
              onClick={() => setShowModal(false)}
              class="btn-ghost text-sm"
            >
              Cancel
            </button>
            <button
              onClick={handleSave}
              disabled={saving || !form.name || !form.dsn}
              class="btn-primary text-sm"
            >
              {saving ? 'Saving...' : 'Add Service'}
            </button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
