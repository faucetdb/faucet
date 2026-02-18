import { useState, useEffect } from 'preact/hooks';
import { Modal } from '../components/Modal';
import { StatusBadge } from '../components/StatusBadge';
import { apiFetch } from '../hooks/useApi';

interface Service {
  id: string;
  name: string;
  type: string;
  host: string;
  port: number;
  database: string;
  username: string;
  status: 'healthy' | 'degraded' | 'error' | 'unknown';
  tables: number;
  created_at: string;
}

const DB_TYPES = [
  { value: 'postgres', label: 'PostgreSQL', port: 5432 },
  { value: 'mysql', label: 'MySQL', port: 3306 },
  { value: 'mssql', label: 'SQL Server', port: 1433 },
  { value: 'snowflake', label: 'Snowflake', port: 443 },
];

const emptyForm = {
  name: '',
  type: 'postgres',
  host: 'localhost',
  port: 5432,
  database: '',
  username: '',
  password: '',
  options: '',
};

export function Services() {
  const [services, setServices] = useState<Service[]>([]);
  const [loading, setLoading] = useState(true);
  const [showModal, setShowModal] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [form, setForm] = useState({ ...emptyForm });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ ok: boolean; message: string } | null>(null);

  useEffect(() => {
    loadServices();
  }, []);

  async function loadServices() {
    setLoading(true);
    try {
      const res = await apiFetch('/api/v1/services');
      setServices(res.resource || []);
    } catch {
      setServices([]);
    } finally {
      setLoading(false);
    }
  }

  function openNew() {
    setForm({ ...emptyForm });
    setEditId(null);
    setError(null);
    setTestResult(null);
    setShowModal(true);
  }

  function openEdit(svc: Service) {
    setForm({
      name: svc.name,
      type: svc.type,
      host: svc.host,
      port: svc.port,
      database: svc.database,
      username: svc.username,
      password: '',
      options: '',
    });
    setEditId(svc.id);
    setError(null);
    setTestResult(null);
    setShowModal(true);
  }

  async function handleTest() {
    setTesting(true);
    setTestResult(null);
    try {
      const res = await apiFetch('/api/v1/services/test', {
        method: 'POST',
        body: {
          type: form.type,
          host: form.host,
          port: form.port,
          database: form.database,
          username: form.username,
          password: form.password,
        },
      });
      setTestResult({ ok: true, message: res.message || 'Connection successful' });
    } catch (err) {
      setTestResult({
        ok: false,
        message: err instanceof Error ? err.message : 'Connection failed',
      });
    } finally {
      setTesting(false);
    }
  }

  async function handleSave() {
    setSaving(true);
    setError(null);
    try {
      const body = {
        name: form.name,
        type: form.type,
        host: form.host,
        port: form.port,
        database: form.database,
        username: form.username,
        password: form.password || undefined,
        options: form.options || undefined,
      };

      if (editId) {
        await apiFetch(`/api/v1/services/${editId}`, { method: 'PUT', body });
      } else {
        await apiFetch('/api/v1/services', { method: 'POST', body });
      }

      setShowModal(false);
      loadServices();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save service');
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete(id: string, name: string) {
    if (!confirm(`Delete service "${name}"? This cannot be undone.`)) return;
    try {
      await apiFetch(`/api/v1/services/${id}`, { method: 'DELETE' });
      loadServices();
    } catch {
      // ignore
    }
  }

  function handleTypeChange(type: string) {
    const dbType = DB_TYPES.find((t) => t.value === type);
    setForm({
      ...form,
      type,
      port: dbType?.port || form.port,
    });
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
            <div key={svc.id} class="card group hover:border-border-default transition-colors">
              <div class="flex items-start justify-between mb-3">
                <div class="flex items-center gap-3">
                  <div class="w-10 h-10 rounded-lg bg-brand/10 flex items-center justify-center shrink-0">
                    <span class="text-sm font-bold text-brand uppercase">{svc.type.slice(0, 2)}</span>
                  </div>
                  <div>
                    <h3 class="text-sm font-semibold text-text-primary">{svc.name}</h3>
                    <p class="text-xs text-text-muted">{svc.type}</p>
                  </div>
                </div>
                <StatusBadge status={svc.status} />
              </div>

              <div class="space-y-1.5 text-xs font-mono text-text-secondary mb-4">
                <div class="flex items-center gap-2">
                  <span class="text-text-muted">host:</span>
                  <span>{svc.host}:{svc.port}</span>
                </div>
                <div class="flex items-center gap-2">
                  <span class="text-text-muted">db:</span>
                  <span>{svc.database}</span>
                </div>
                <div class="flex items-center gap-2">
                  <span class="text-text-muted">tables:</span>
                  <span>{svc.tables}</span>
                </div>
              </div>

              <div class="flex items-center gap-2 pt-3 border-t border-border-subtle">
                <button
                  onClick={() => openEdit(svc)}
                  class="btn-ghost text-xs py-1.5 px-3"
                >
                  Edit
                </button>
                <a
                  href={`/schema?service=${svc.name}`}
                  class="btn-ghost text-xs py-1.5 px-3"
                >
                  Schema
                </a>
                <button
                  onClick={() => handleDelete(svc.id, svc.name)}
                  class="btn-ghost text-xs py-1.5 px-3 text-error hover:text-error ml-auto"
                >
                  Delete
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Add/Edit Modal */}
      <Modal
        isOpen={showModal}
        onClose={() => setShowModal(false)}
        title={editId ? 'Edit Service' : 'Add Service'}
        width="max-w-xl"
      >
        <div class="space-y-4">
          {error && (
            <div class="p-3 rounded-lg bg-error/10 border border-error/20 text-sm text-error">
              {error}
            </div>
          )}

          <div class="grid grid-cols-2 gap-4">
            <div class="col-span-2">
              <label class="block text-sm font-medium text-text-secondary mb-1.5">Service Name</label>
              <input
                type="text"
                class="input w-full"
                placeholder="my-database"
                value={form.name}
                onInput={(e) => setForm({ ...form, name: (e.target as HTMLInputElement).value })}
              />
            </div>

            <div class="col-span-2">
              <label class="block text-sm font-medium text-text-secondary mb-1.5">Database Type</label>
              <select
                class="input w-full"
                value={form.type}
                onChange={(e) => handleTypeChange((e.target as HTMLSelectElement).value)}
              >
                {DB_TYPES.map((t) => (
                  <option key={t.value} value={t.value}>{t.label}</option>
                ))}
              </select>
            </div>

            <div>
              <label class="block text-sm font-medium text-text-secondary mb-1.5">Host</label>
              <input
                type="text"
                class="input w-full font-mono text-sm"
                placeholder="localhost"
                value={form.host}
                onInput={(e) => setForm({ ...form, host: (e.target as HTMLInputElement).value })}
              />
            </div>

            <div>
              <label class="block text-sm font-medium text-text-secondary mb-1.5">Port</label>
              <input
                type="number"
                class="input w-full font-mono text-sm"
                value={form.port}
                onInput={(e) => setForm({ ...form, port: parseInt((e.target as HTMLInputElement).value) || 0 })}
              />
            </div>

            <div class="col-span-2">
              <label class="block text-sm font-medium text-text-secondary mb-1.5">Database Name</label>
              <input
                type="text"
                class="input w-full font-mono text-sm"
                placeholder="mydb"
                value={form.database}
                onInput={(e) => setForm({ ...form, database: (e.target as HTMLInputElement).value })}
              />
            </div>

            <div>
              <label class="block text-sm font-medium text-text-secondary mb-1.5">Username</label>
              <input
                type="text"
                class="input w-full font-mono text-sm"
                placeholder="postgres"
                value={form.username}
                onInput={(e) => setForm({ ...form, username: (e.target as HTMLInputElement).value })}
              />
            </div>

            <div>
              <label class="block text-sm font-medium text-text-secondary mb-1.5">Password</label>
              <input
                type="password"
                class="input w-full font-mono text-sm"
                placeholder={editId ? '(unchanged)' : ''}
                value={form.password}
                onInput={(e) => setForm({ ...form, password: (e.target as HTMLInputElement).value })}
              />
            </div>

            <div class="col-span-2">
              <label class="block text-sm font-medium text-text-secondary mb-1.5">
                Connection Options <span class="text-text-muted font-normal">(optional)</span>
              </label>
              <input
                type="text"
                class="input w-full font-mono text-sm"
                placeholder="sslmode=require"
                value={form.options}
                onInput={(e) => setForm({ ...form, options: (e.target as HTMLInputElement).value })}
              />
            </div>
          </div>

          {/* Test result */}
          {testResult && (
            <div
              class={`p-3 rounded-lg text-sm ${
                testResult.ok
                  ? 'bg-success/10 border border-success/20 text-success'
                  : 'bg-error/10 border border-error/20 text-error'
              }`}
            >
              {testResult.message}
            </div>
          )}

          {/* Actions */}
          <div class="flex items-center justify-between pt-4 border-t border-border-subtle">
            <button
              onClick={handleTest}
              disabled={testing}
              class="btn-secondary text-sm"
            >
              {testing ? 'Testing...' : 'Test Connection'}
            </button>
            <div class="flex items-center gap-2">
              <button
                onClick={() => setShowModal(false)}
                class="btn-ghost text-sm"
              >
                Cancel
              </button>
              <button
                onClick={handleSave}
                disabled={saving || !form.name || !form.database}
                class="btn-primary text-sm"
              >
                {saving ? 'Saving...' : editId ? 'Update Service' : 'Add Service'}
              </button>
            </div>
          </div>
        </div>
      </Modal>
    </div>
  );
}
